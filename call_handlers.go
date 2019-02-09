package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

func listCalls(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	var calls []Call
	err = pg.Select(&calls, `
SELECT *
FROM calls
WHERE contract_id = $1
ORDER BY time DESC
LIMIT 20
        `, ctid)
	if err == sql.ErrNoRows {
		calls = make([]Call, 0)
	} else if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch calls")
		jsonError(w, "", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: calls})
}

func prepareCall(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	call := &Call{}
	err := json.NewDecoder(r.Body).Decode(call)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse call json.")
		jsonError(w, "", 400)
		return
	}
	call.ContractId = ctid
	call.Id = cuid.Slug()
	logger = logger.With().Str("callid", call.Id).Logger()

	// verify call is valid as best as possible
	if len(call.Method) == 0 || call.Method[0] == '_' {
		logger.Warn().Err(err).Str("method", call.Method).Msg("invalid method")
		jsonError(w, "invalid method", 400)
		return
	}

	call.calcCosts()
	err = call.getInvoice()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to make invoice.")
		jsonError(w, "failed to make invoice, please try again", 500)
		return
	}

	_, err = call.saveOnRedis()
	if err != nil {
		logger.Warn().Err(err).Interface("call", call).
			Msg("failed to save call on redis")
		jsonError(w, "", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: call})
}

func getCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	logger := log.With().Str("callid", callid).Logger()

	call := &Call{}
	err = pg.Get(call, `
SELECT * FROM calls WHERE id = $1
    `, callid)
	if err == sql.ErrNoRows {
		call, err = callFromRedis(callid)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to fetch call from redis")
			jsonError(w, "couldn't find call "+callid+", it may have expired", 404)
			return
		}
	} else if err != nil {
		// it's a database error
		logger.Warn().Err(err).Msg("database error fetching call")
		jsonError(w, "failed to fetch call "+callid, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: call})
}

func makeCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	logger := log.With().Str("callid", callid).Logger()

	call, err := callFromRedis(callid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		jsonError(w, "couldn't find call "+callid+", it may have expired.", 404)
		return
	}

	logger.Info().Interface("call", call).Msg("call being made")
	label := s.ServiceId + "." + call.ContractId + "." + callid
	_, err = checkPayment(label, call.Cost+call.Satoshis*1000)
	if err != nil {
		logger.Warn().Err(err).Msg("payment check failed")
		jsonError(w, "payment check failed", 402)
		return
	}

	// proceed to run the call
	txn, err := pg.BeginTxx(context.TODO(),
		&sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		logger.Warn().Err(err).Msg("transaction start failed")
		jsonError(w, "", 500)
		return
	}
	defer txn.Rollback()
	returnedValue, err := call.runCall(txn)
	if err != nil {
		logger.Warn().Err(err).Str("ctid", call.ContractId).Msg("failed to run call")
		jsonError(w, "failed to run call", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: returnedValue})
}
