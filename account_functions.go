package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/fiatjaf/etleneum/types"
)

func getAccountSecret(account string) string {
	hash := sha256.Sum256([]byte(account + "-" + s.SecretKey))
	return hex.EncodeToString(hash[:])
}

func hmacCall(call *types.Call) []byte {
	mac := hmac.New(sha256.New, []byte(getAccountSecret(call.Caller)))
	mac.Write([]byte(callHmacString(call)))
	return mac.Sum(nil)
}

func callHmacString(call *types.Call) (res string) {
	res = fmt.Sprintf("%s:%s:%d,", call.ContractId, call.Method, call.Msatoshi)

	var payload map[string]interface{}
	json.Unmarshal(call.Payload, &payload)

	// sort keys
	keys := make([]string, len(payload))
	i := 0
	for k, _ := range payload {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	// add key-values
	for _, k := range keys {
		v := payload[k]
		res += fmt.Sprintf("%s=%v", k, v)
		res += ","
	}

	return
}
