package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/go-lnurl"
	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
	"github.com/tidwall/gjson"
)

func lnurlCallMetadata(call *types.Call) string {
	desc := fmt.Sprintf(`Call method "%s" on contract "%s" with payload %v`,
		call.Method, call.ContractId, call.Payload)
	if call.Caller != "" {
		desc += fmt.Sprintf(" on behalf of account %s", call.Caller)
	}
	desc += fmt.Sprintf(" including %d msatoshi.", call.Msatoshi)

	jmetadata, _ := json.Marshal([][]string{
		[]string{
			"text/plain",
			desc,
		},
	})

	return string(jmetadata)
}

func lnurlPayParams(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ctid := vars["ctid"]
	method := vars["method"]

	// if not given default to 1msat and let the person choose on lnurl-pay UI
	msatoshi, _ := strconv.ParseInt(vars["msatoshi"], 10, 64)
	if msatoshi == 0 {
		msatoshi = 1
	}

	logger := log.With().Str("ctid", ctid).Bool("lnurl", true).Logger()

	qs := r.URL.Query()

	// if the user has a personal token and wants to make an authenticated call
	var caller string
	if token := qs.Get("_token"); token != "" {
		if account, ok := accountFromToken(token); ok {
			caller = account
		} else {
			logger.Warn().Str("token", token).Msg("token mismatch")
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid token."))
			return
		}
	}

	// payload comes as query parameters
	payload := make(map[string]interface{})
	for k, _ := range qs {
		v := gjson.Parse(qs.Get(k)).Value()
		if v == nil {
			v = qs.Get(k)
		}
		payload[k] = v
	}
	jpayload, _ := json.Marshal(payload)

	call := &types.Call{
		Id:         "r" + cuid.Slug(),
		ContractId: ctid,
		Method:     method,
		Msatoshi:   msatoshi,
		Payload:    []byte(jpayload),
		Caller:     caller,
	}
	call.Cost = getCallCosts(*call)
	logger = logger.With().Str("callid", call.Id).Logger()

	// verify call is valid as best as possible
	if len(call.Method) == 0 || call.Method[0] == '_' {
		logger.Warn().Err(err).Str("method", call.Method).Msg("invalid method")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid method '" + call.Method + "'."))
		return
	}

	_, err = saveCallOnRedis(*call)
	if err != nil {
		logger.Error().Err(err).Interface("call", call).
			Msg("failed to save call on redis")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to save call data."))
		return
	}

	min := call.Msatoshi
	max := call.Msatoshi
	if call.Msatoshi == 1 {
		min = 1
		max = 1000000
	}

	json.NewEncoder(w).Encode(lnurl.LNURLPayResponse1{
		Tag:             "payRequest",
		Callback:        s.ServiceURL + "/lnurl/call/" + call.Id,
		EncodedMetadata: lnurlCallMetadata(call),
		MinSendable:     min,
		MaxSendable:     max,
	})
}

func lnurlPayValues(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	callid := vars["callid"]
	msatoshi, _ := strconv.ParseInt(vars["msatoshi"], 10, 64)

	logger := log.With().Str("callid", callid).Logger()

	call, err := callFromRedis(callid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to fetch call data."))
		return
	}
	descriptionHash := sha256.Sum256([]byte(lnurlCallMetadata(call)))

	pr, err := makeInvoice(call.Id, "", &descriptionHash, msatoshi, call.Cost)
	if err != nil {
		logger.Error().Err(err).Msg("translate invoice")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to fetch call data."))
		return
	}

	// update the call saved on redis so we can check values paid later
	if msatoshi != 0 && call.Msatoshi != msatoshi {
		call.Msatoshi = msatoshi

		_, err = saveCallOnRedis(*call)
		if err != nil {
			logger.Error().Err(err).Interface("call", call).
				Msg("failed to save call on redis after lnurl-pay step 2")
			json.NewEncoder(w).Encode(
				lnurl.ErrorResponse("Failed to save call with new amount."))
			return
		}
	}

	json.NewEncoder(w).Encode(lnurl.LNURLPayResponse2{
		Routes:        make([][]lnurl.RouteInfo, 0),
		PR:            pr,
		SuccessAction: nil,
	})
}
