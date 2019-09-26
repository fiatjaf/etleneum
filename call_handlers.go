package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/fiatjaf/etleneum/types"
	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

func listCalls(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	var calls []types.Call
	err = pg.Select(&calls, `
SELECT `+types.CALLFIELDS+`
FROM calls
WHERE contract_id = $1
ORDER BY time DESC
LIMIT 20
        `, ctid)
	if err == sql.ErrNoRows {
		calls = make([]types.Call, 0)
	} else if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch calls")
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
	logger = logger.With().Str("callid", call.Id).Logger()

	// if the user has authorized and want to make an authenticated call
	if session := r.URL.Query().Get("session"); session != "" {
		if accountId, err := rds.Get("auth-session:" + session).Result(); err != nil {
			log.Warn().Err(err).Str("session", session).Msg("failed to get account for authenticated session")
			jsonError(w, "failed to get account for authenticated session", 400)
			return
		} else {
			call.Caller = accountId
		}
	}

	// verify call is valid as best as possible
	if len(call.Method) == 0 || call.Method[0] == '_' {
		logger.Warn().Err(err).Str("method", call.Method).Msg("invalid method")
		jsonError(w, "invalid method", 400)
		return
	}

	calcCallCosts(call)
	err = getCallInvoice(call)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to make invoice.")
		jsonError(w, "failed to make invoice, please try again", 500)
		return
	}

	_, err = saveCallOnRedis(*call)
	if err != nil {
		logger.Warn().Err(err).Interface("call", call).
			Msg("failed to save call on redis")
		jsonError(w, "failed to save prepared call", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: call})
}

func getCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	logger := log.With().Str("callid", callid).Logger()

	call := &types.Call{}
	err = pg.Get(call, `
SELECT `+types.CALLFIELDS+`
FROM calls WHERE id = $1
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
		logger.Warn().Err(err).Interface("call", call).
			Msg("failed to save patched call on redis")
		jsonError(w, "failed to save patched call", 500)
		return
	}

	json.NewEncoder(w).Encode(Result{Ok: true, Value: call})
}
