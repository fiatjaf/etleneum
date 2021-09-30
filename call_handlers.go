package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/fiatjaf/etleneum/data"
	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

func prepareCall(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	call := &data.Call{}
	err := json.NewDecoder(r.Body).Decode(call)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse call json")
		jsonError(w, "failed to parse json", 400)
		return
	}
	call.ContractId = ctid
	call.Id = "r" + cuid.Slug()
	call.Cost = getCallCosts(*call, false)
	logger = logger.With().Str("callid", call.Id).Str("method", call.Method).Logger()
	var useBalance bool

	// if the user has authorized and want to make an authenticated call
	if session := r.URL.Query().Get("session"); session != "" {
		accountId := rds.Get("auth-session:" + session).Val()
		if accountId == "" {
			log.Warn().Err(err).Str("session", session).
				Msg("failed to get account for authenticated session")
			jsonError(w, "failed to get account for authenticated session", 400)
			return
		}

		// here we have an account successfully fetched
		call.Caller = accountId

		// if the user wants to pay for the call using funds from his balance
		if _, ok := r.URL.Query()["use-balance"]; ok {
			balance := data.GetAccountBalance(accountId)

			if call.Msatoshi+call.Cost > int64(float64(balance)*1.004) {
				log.Warn().Err(err).Interface("account", accountId).
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
		logger.Warn().Err(err).Msg("invalid method")
		jsonError(w, "invalid method", 400)
		return
	}

	// if useBalance then we try to run the call already
	// and pay with funds from account balance
	if useBalance {
		data.Start()
		logger.Info().Interface("call", call).Msg("call being made with balance funds")

		err = runCallGlobal(call, true)
		if err != nil {
			logger.Warn().Err(err).Str("payload", string(call.Payload)).
				Msg("failed to run call")
			data.Abort()
			jsonError(w, "failed to run call", 400)
			dispatchContractEvent(call.ContractId,
				ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi,
					err.Error(), "runtime"}, "call-error")
			return
		}

		// commit
		data.Finish("call " + call.Id + " executed on contract " + call.ContractId + ".")

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
		json.NewEncoder(w).Encode(Result{
			Ok: true,
			Value: struct {
				*data.Call
				Invoice string `json:"invoice"`
			}{Call: call, Invoice: invoice},
		})
	}
}

func getCall(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	callid := mux.Vars(r)["callid"]

	call, err := data.GetCall(ctid, callid)
	if err != nil {
		// it's a database error
		log.Warn().Err(err).Str("callid", callid).Msg("database error fetching call")
		jsonError(w, "database error", 500)
		return
	}

	if call == nil {
		// couldn't find on database, maybe it's a temporary contract?
		call, err = callFromRedis(callid)
		if err != nil {
			log.Warn().Err(err).Str("callid", callid).
				Msg("failed to fetch fetch prepared call from redis")
			jsonError(w, "failed to fetch prepared call", 404)
			return
		}
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
