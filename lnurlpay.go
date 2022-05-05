package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	_ "image/png"
	"net/http"
	"strconv"

	"github.com/fiatjaf/etleneum/data"
	"github.com/fiatjaf/go-lnurl"
	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
	"github.com/tidwall/gjson"
)

func lnurlCallMetadata(call *data.Call, fixedAmount bool) string {
	desc := fmt.Sprintf(`Call method "%s" on contract "%s" with payload %s`,
		call.Method, call.ContractId, string(call.Payload))
	if call.Caller != "" {
		desc += fmt.Sprintf(" on behalf of account %s", call.Caller)
	}
	if fixedAmount {
		desc += fmt.Sprintf(" including %d msatoshi.", call.Msatoshi)
	} else {
		desc += " with variadic amount."
	}

	metadata := [][]string{{"text/plain", desc}}
	if imageb64, err := generateLnurlImage(call.ContractId, call.Method); err == nil {
		metadata = append(metadata, []string{"image/png;base64", imageb64})
	} else {
		log.Warn().Err(err).Msg("error generating image for lnurl")
	}
	jmetadata, _ := json.Marshal(metadata)

	return string(jmetadata)
}

func lnurlPayParams(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ctid := vars["ctid"]
	method := vars["method"]
	msatoshi, _ := strconv.ParseInt(vars["msatoshi"], 10, 64)

	logger := log.With().
		Str("ctid", ctid).
		Str("url", r.URL.String()).
		Bool("lnurl", true).
		Logger()

	qs := r.URL.Query()

	// fixed minSendable, maxSendable
	var defaultMinSendable int64 = 1
	var defaultMaxSendable int64 = 1000000000
	var err error
	if min := qs.Get("_minsendable"); min != "" {
		defaultMinSendable, err = strconv.ParseInt(min, 10, 64)
		if err != nil {
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("_minsendable param is invalid."))
			return
		}
		qs.Del("_minsendable")
	}
	if max := qs.Get("_maxsendable"); max != "" {
		defaultMaxSendable, err = strconv.ParseInt(max, 10, 64)
		if err != nil {
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("_maxsendable param is invalid."))
			return
		}
		qs.Del("_maxsendable")
	}

	// payload comes as query parameters
	payload := make(map[string]interface{})
	for k := range qs {
		if k[0] == '_' {
			continue
		}

		v := qs.Get(k)
		if gjson.Valid(v) {
			payload[k] = gjson.Parse(v).Value()
		} else {
			payload[k] = v
		}

	}
	jpayload, _ := json.Marshal(payload)

	call := &data.Call{
		Id:         "r" + cuid.Slug(),
		ContractId: ctid,
		Method:     method,
		Msatoshi:   msatoshi,
		Payload:    []byte(jpayload),
	}
	call.Cost = getCallCosts(*call, false)

	// if the user has hmac'ed this call we set them as the caller
	if account := qs.Get("_account"); account != "" {
		mac, _ := hex.DecodeString(qs.Get("_hmac"))
		call.Caller = account // assume correct

		// then verify
		if !hmac.Equal(mac, hmacCall(call)) {
			logger.Warn().Str("hmac", hex.EncodeToString(mac)).
				Str("expected", hex.EncodeToString(hmacCall(call))).
				Str("serialized", callHmacString(call)).
				Msg("hmac mismatch")
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid HMAC."))
			return
		}
	}

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

	var min, max int64
	var encodedMetadata string
	if call.Msatoshi == 0 && vars["msatoshi"] != "0" {
		// if amount is not given let the person choose on lnurl-pay UI
		min = defaultMinSendable
		max = defaultMaxSendable
		encodedMetadata = lnurlCallMetadata(call, false)
	} else {
		// otherwise make the lnurl params be the full main_price + cost
		min = call.Msatoshi + call.Cost
		max = call.Msatoshi + call.Cost
		encodedMetadata = lnurlCallMetadata(call, true)
	}

	json.NewEncoder(w).Encode(lnurl.LNURLPayResponse1{
		Tag:             "payRequest",
		Callback:        s.ServiceURL + "/lnurl/call/" + call.Id,
		EncodedMetadata: encodedMetadata,
		MinSendable:     min,
		MaxSendable:     max,
	})
}

func lnurlPayValues(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	msatoshi, _ := strconv.ParseInt(r.URL.Query().Get("amount"), 10, 64)

	logger := log.With().Str("callid", callid).Logger()

	call, err := callFromRedis(callid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to fetch call data."))
		return
	}

	var encodedMetadata string
	var lastHopFee int64

	// update the call saved on redis so we can check values paid later.
	// this is only needed if the lnurl-pay params sent before were variable
	//   and the user has chosen them in the wallet (i.e., they were not hardcoded
	//   in the lnurl itself.
	if call.Msatoshi == 0 && msatoshi != (call.Msatoshi+call.Cost) {
		// to make the lnurl wallet happy, we'll generate an invoice for
		//   the exact msatoshi amount chosen in the screen, costs will be
		//   appended as fees in the last hop shadow channel.
		call.Msatoshi = msatoshi
		call.Cost = getCallCosts(*call, true)
		lastHopFee = call.Cost

		_, err = saveCallOnRedis(*call)
		if err != nil {
			logger.Error().Err(err).Interface("call", call).
				Msg("failed to save call on redis after lnurl-pay step 2")
			json.NewEncoder(w).Encode(
				lnurl.ErrorResponse("Failed to save call with new amount."))
			return
		}

		encodedMetadata = lnurlCallMetadata(call, false)
	} else {
		encodedMetadata = lnurlCallMetadata(call, true)
		lastHopFee = 0
	}

	descriptionHash := sha256.Sum256([]byte(encodedMetadata))
	pr, err := makeInvoice(false, call.ContractId, call.Id, "", &descriptionHash, msatoshi, lastHopFee)
	if err != nil {
		logger.Error().Err(err).Msg("translate invoice")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Error translating invoice."))
		return
	}

	json.NewEncoder(w).Encode(lnurl.LNURLPayResponse2{
		Routes: make([][]lnurl.RouteInfo, 0),
		PR:     pr,
		// SuccessAction: lnurl.Action("Call Details",
		// 	"https://github.com/etleneum/database/tree/master/contracts/"+call.ContractId+"/calls/"+call.Id[1:2]+"/"+call.Id,
		// ),
		Disposable: lnurl.FALSE,
	})
}
