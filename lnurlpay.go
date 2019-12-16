package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/go-lnurl"
	decodepay "github.com/fiatjaf/ln-decodepay"
	"github.com/gorilla/mux"
	"github.com/lightningnetwork/lnd/zpay32"
	"github.com/lucsky/cuid"
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

func translateInvoice(bolt11 string, descriptionHash [32]byte) (string, error) {
	invoice, err := zpay32.Decode(bolt11, decodepay.ChainFromCurrency(bolt11[2:]))
	if err != nil {
		return "", err
	}
	invoice.Description = nil
	invoice.Destination = nil
	invoice.DescriptionHash = &descriptionHash

	privKey, err := ln.GetPrivateKey()
	if err != nil {
		return "", err
	}

	return invoice.Encode(zpay32.MessageSigner{
		SignCompact: func(hash []byte) ([]byte, error) {
			return btcec.SignCompact(btcec.S256(), privKey, hash, true)
		},
	})
}

func lnurlPayParams(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ctid := vars["ctid"]
	method := vars["method"]
	msatoshi, err := strconv.Atoi(vars["msatoshi"])
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("msatoshi is '" + vars["msatoshi"] + "', must be an integer"))
		return
	}
	logger := log.With().Str("ctid", ctid).Bool("lnurl", true).Logger()

	qs := r.URL.Query()

	// if the user has a personal token and wants to make an authenticated call
	var caller string
	if token := qs.Get("_token"); token != "" {
		rtoken, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			logger.Warn().Err(err).Str("token", token).Msg("token base64")
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid token."))
			return
		}
		spl := strings.Split(string(rtoken), ":")
		if len(spl) != 2 {
			logger.Warn().Str("rtoken", string(rtoken)).Msg("token split")
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid token."))
			return
		}
		account := spl[0]
		passkey := spl[1]
		hash := sha256.Sum256([]byte(account + "~" + s.SecretKey))
		if passkey == hex.EncodeToString(hash[:]) {
			caller = account
		} else {
			logger.Warn().Str("rtoken", string(rtoken)).Msg("token mismatch")
			json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid token."))
			return
		}
	}

	// payload comes as query parameters
	payload := make(map[string]interface{})
	for k, v := range qs {
		if f, err := strconv.ParseFloat(v[0], 10); err == nil {
			payload[k] = f
		} else {
			payload[k] = v[0]
		}
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
	logger = logger.With().Str("callid", call.Id).Logger()

	// verify call is valid as best as possible
	if len(call.Method) == 0 || call.Method[0] == '_' {
		logger.Warn().Err(err).Str("method", call.Method).Msg("invalid method")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Invalid method '" + call.Method + "'."))
		return
	}

	label, err := setCallInvoice(call)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to make invoice.")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to make invoice."))
		return
	}

	if s.FreeMode {
		// wait 10 seconds and notify this payment was received
		go func() {
			time.Sleep(10 * time.Second)
			handlePaymentReceived(label, lnurl.RandomK1())
		}()
	}

	_, err = saveCallOnRedis(*call)
	if err != nil {
		logger.Error().Err(err).Interface("call", call).
			Msg("failed to save call on redis")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to save call data."))
		return
	}

	json.NewEncoder(w).Encode(lnurl.LNURLPayResponse1{
		Tag:             "payRequest",
		Callback:        s.ServiceURL + "/lnurl/call/" + call.Id,
		EncodedMetadata: lnurlCallMetadata(call),
		MinSendable:     int64(call.Cost + call.Msatoshi),
		MaxSendable:     int64(call.Cost + call.Msatoshi),
	})
}

func lnurlPayValues(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	logger := log.With().Str("callid", callid).Logger()

	call, err := callFromRedis(callid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to fetch call data."))
		return
	}

	descriptionHash := sha256.Sum256([]byte(lnurlCallMetadata(call)))

	pr, err := translateInvoice(call.Bolt11, descriptionHash)
	if err != nil {
		logger.Error().Err(err).Msg("translate invoice")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("Failed to fetch call data."))
		return
	}

	json.NewEncoder(w).Encode(lnurl.LNURLPayResponse2{
		Routes:        make([][]lnurl.RouteInfo, 0),
		PR:            pr,
		SuccessAction: nil,
	})
}
