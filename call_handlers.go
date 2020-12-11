package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/fiatjaf/etleneum/types"
	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

func listCalls(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "50"
	}

	calls := make([]types.Call, 0)
	err = pg.Select(&calls, `
SELECT `+types.CALLFIELDS+`
FROM calls
WHERE contract_id = $1
   OR $1 IN (SELECT to_contract FROM internal_transfers it WHERE calls.id = it.call_id)
ORDER BY time DESC
LIMIT $2
        `, ctid, limit)
	if err == sql.ErrNoRows {
		calls = make([]types.Call, 0)
	} else if err != nil {
		logger.Error().Err(err).Msg("failed to fetch calls")
		jsonError(w, "failed to fetch calls", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: calls})
}

func prepareCall(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	call := &types.Call{}
	err := json.NewDecoder(r.Body).Decode(call)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse call json")
		jsonError(w, "failed to parse json", 400)
		return
	}
	call.ContractId = ctid
	call.Id = "r" + cuid.Slug()
	call.Cost = getCallCosts(*call, false)
	logger = logger.With().Str("callid", call.Id).Logger()
	var useBalance bool

	// if the user has authorized and want to make an authenticated call
	if session := r.URL.Query().Get("session"); session != "" {
		accountId := rds.Get("auth-session:" + session).Val()
		account, err := loadAccount(accountId)
		if err != nil {
			log.Warn().Err(err).Str("session", session).
				Msg("failed to get account for authenticated session")
			jsonError(w, "failed to get account for authenticated session", 400)
			return
		}

		// here we have an account successfully fetched
		call.Caller = account.Id

		// if the user wants to pay for the call using funds from his balance
		if _, ok := r.URL.Query()["use-balance"]; ok {
			if call.Msatoshi+call.Cost > int64(float64(account.Balance)*1.004) {
				log.Warn().Err(err).Interface("account", account).
					Interface("call", call).
					Msg("not enough funds")
				jsonError(w, "not enough funds to use balance", 402)
				return
			}

			useBalance = true
		}
	}

	// verify call is valid as best as possible
	if len(call.Method) == 0 || call.Method[0] == '_' {
		logger.Warn().Err(err).Str("method", call.Method).Msg("invalid method")
		jsonError(w, "invalid method", 400)
		return
	}

	// if useBalance then we try to run the call already
	// and pay with funds from account balance
	if useBalance {
		// first create the transaction
		txn, err := pg.BeginTxx(context.TODO(),
			&sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			logger.Warn().Err(err).Msg("transaction start failed")
			jsonError(w, "failed to start transaction", 500)
			dispatchContractEvent(call.ContractId,
				ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi,
					err.Error(), "internal"}, "call-error")
			return
		}
		defer txn.Rollback()

		logger.Info().Interface("call", call).Msg("call being made with balance funds")

		err = runCall(call, txn, true)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to run call")
			jsonError(w, "failed to run call", 400)
			dispatchContractEvent(call.ContractId,
				ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi,
					err.Error(), "runtime"}, "call-error")
			return
		}

		// commit
		err = txn.Commit()
		if err != nil {
			log.Warn().Err(err).Str("call.id", call.Id).Msg("failed to commit call")
			jsonError(w, "failed to commit call", 400)
			dispatchContractEvent(call.ContractId,
				ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi,
					"failed to commit call", "internal"}, "call-error")
			return
		}

		// call was successful
		dispatchContractEvent(call.ContractId,
			ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi, "", ""},
			"call-made")
	} else {
		// useBalance = false, so we just prepare the call and show an invoice
		// make an invoice and save the prepared call
		invoice, err := makeInvoice(
			s.FreeMode,
			call.ContractId,
			call.Id,
			s.ServiceId+" "+call.Method+" ["+call.ContractId+"]["+call.Id+"]",
			nil,
			call.Msatoshi+call.Cost,
			0,
		)
		if err != nil {
			logger.Error().Err(err).Msg("failed to make invoice.")
			jsonError(w, "failed to make invoice, please try again", 500)
			return
		}
		if s.FreeMode {
			// wait 5 seconds and notify this payment was received
			go func() {
				time.Sleep(5 * time.Second)
				callPaymentReceived(call.Id, call.Msatoshi+call.Cost)
			}()
		}

		_, err = saveCallOnRedis(*call)
		if err != nil {
			logger.Error().Err(err).Interface("call", call).
				Msg("failed to save call on redis")
			jsonError(w, "failed to save prepared call", 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Result{Ok: true, Value: types.StuffBeingCreated{
			Id:      call.Id,
			Invoice: invoice,
		}})
	}
}

func getCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	logger := log.With().Str("callid", callid).Logger()

	call := &types.Call{}
	err = pg.Get(call, `
SELECT `+types.CALLFIELDS+`, coalesce(diff, '') AS diff
FROM calls
WHERE id = $1
ORDER BY time DESC
LIMIT $2
    `, callid, limit)
	if err == sql.ErrNoRows {
		call, err = callFromRedis(callid)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to fetch call from redis")
			jsonError(w, "couldn't find call "+callid+", it may have expired", 404)
			return
		}
	} else if err != nil {
		// it's a database error
		logger.Error().Err(err).Msg("database error fetching call")
		jsonError(w, "failed to fetch call "+callid, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: call})
}

// changes call payload after being prepared
func patchCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	logger := log.With().Str("callid", callid).Logger()

	call, err := callFromRedis(callid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		jsonError(w, "couldn't find call "+callid+", it may have expired.", 404)
		return
	}

	// authenticated calls can only be modified by an authenticated session
	if call.Caller != "" {
		if session := r.URL.Query().Get("session"); session != "" {
			accountId, err := rds.Get("auth-session:" + session).Result()
			if err != nil || accountId != call.Caller {
				jsonError(w, "only the author can patch an authenticated call.", 401)
				return
			}
		}
	}

	patch := make(map[string]interface{})
	err = json.NewDecoder(r.Body).Decode(&patch)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse patch json")
		jsonError(w, "failed to parse json", 400)
		return
	}

	payload := make(map[string]interface{})
	err = json.Unmarshal(call.Payload, &payload)
	for k, v := range patch {
		payload[k] = v
	}
	jpayload, _ := json.Marshal(payload)
	call.Payload.UnmarshalJSON(jpayload)

	_, err = saveCallOnRedis(*call)
	if err != nil {
		logger.Error().Err(err).Interface("call", call).
			Msg("failed to save patched call on redis")
		jsonError(w, "failed to save patched call", 500)
		return
	}

	json.NewEncoder(w).Encode(Result{Ok: true, Value: call})
}
