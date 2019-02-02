package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

func listContracts(w http.ResponseWriter, r *http.Request) {
	var contracts []Contract
	err = pg.Select(&contracts, `
SELECT id, name, readme FROM (
  SELECT id, name, readme, created_at,
    (SELECT max(time) FROM calls WHERE contract_id = c.id) AS lastcalltime
  FROM contracts AS c
) AS x
ORDER BY lastcalltime DESC, created_at DESC
`)
	if err == sql.ErrNoRows {
		contracts = make([]Contract, 0)
	} else if err != nil {
		log.Warn().Err(err).Msg("failed to fetch contracts")
		jsonError(w, "", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: contracts})
}

func prepareContract(w http.ResponseWriter, r *http.Request) {
	// making a contract only saves it temporarily.
	// the contract can be inspected only by its creator.
	// once the creator knows everything is right, he can call init.
	ct := &Contract{}
	err := json.NewDecoder(r.Body).Decode(ct)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse contract json.")
		jsonError(w, "", 400)
		return
	}
	ct.Id = cuid.Slug()

	ct.calcCosts()
	err = ct.getInvoice()
	if err != nil {
		log.Warn().Err(err).Msg("failed to make invoice.")
		jsonError(w, "", 500)
		return
	}

	_, err = ct.saveOnRedis()
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		jsonError(w, "", 500)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: ct})
}

func getContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct := &Contract{}
	err = pg.Get(ct, `
SELECT contract.*,
  coalesce(sum(satoshis - paid), 0) AS funds
FROM contracts AS contract
LEFT OUTER JOIN calls AS call ON call.contract_id = contract.id
WHERE contract.id = $1
GROUP BY contract.id`,
		ctid)
	if err == sql.ErrNoRows {
		// couldn't find on database, maybe it's a temporary contract?
		ct, err = contractFromRedis(ctid)
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to fetch fetch contract from redis")
			jsonError(w, "", 404)
			return
		}
		err = ct.getInvoice()
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to get/make invoice")
			jsonError(w, "", 500)
			return
		}
	} else if err != nil {
		// it's a database error
		log.Warn().Err(err).Str("ctid", ctid).Msg("database error fetching contract")
		jsonError(w, "", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: ct})
}

func makeContract(w http.ResponseWriter, r *http.Request) {
	// init is a special call that enables a contract.
	// it has a fixed cost and its payload is the initial state of the contract.
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	ct, err := contractFromRedis(ctid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch contract from redis")
		jsonError(w, "couldn't find contract "+ctid+", it may have expired", 404)
		return
	}

	err = checkPayment(s.ServiceId+"."+ctid, ct.Cost)
	if err != nil {
		logger.Warn().Err(err).Msg("payment check failed")
		jsonError(w, "payment check failed", 402)
		return
	}

	// initiate transaction
	txn, err := pg.BeginTxx(context.TODO(),
		&sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		logger.Warn().Err(err).Msg("transaction start failed")
		jsonError(w, "", 500)
		return
	}
	defer txn.Rollback()

	// create initial contract
	_, err = txn.Exec(`
INSERT INTO contracts (id, name, readme, code, cost, state)
VALUES ($1, $2, $3, $4, $5, '{}')
    `, ct.Id, ct.Name, ct.Readme, ct.Code, ct.Cost)
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).
			Msg("failed to save contract on database")
		jsonError(w, "", 500)
		return
	}

	// instantiate call
	call := Call{
		ContractId: ct.Id,
		Id:         ct.Id, // same
		Method:     "__init__",
		Payload:    []byte{},
		Cost:       0,
	}

	_, err = call.runCall(txn)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to run call")
		jsonError(w, "failed to run call", 500)
		return
	}

	// saved. delete from redis.
	rds.Del("contract:" + ctid)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: nil})
}

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
	err = checkPayment(label, call.Cost+call.Satoshis*1000)
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
