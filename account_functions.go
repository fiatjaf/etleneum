package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/fiatjaf/etleneum/data"
)

func getAccountSecret(account string) string {
	hash := sha256.Sum256([]byte(account + "-" + s.SecretKey))
	return hex.EncodeToString(hash[:])
}

func hmacCall(call *data.Call) []byte {
	mac := hmac.New(sha256.New, []byte(getAccountSecret(call.Caller)))
	mac.Write([]byte(callHmacString(call)))
	return mac.Sum(nil)
}

func callHmacString(call *data.Call) (res string) {
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

func hmacAccount(accountId string) string {
	mac := hmac.New(sha256.New, []byte(s.SecretKey))
	mac.Write([]byte(accountId))
	return hex.EncodeToString(mac.Sum(nil))
}

func getStaticLNURLWithdraw(accountId string) string {
	return fmt.Sprintf("%s/lnurl/withdraw?acct=%s&hmac=%s",
		s.ServiceURL, accountId, hmacAccount(accountId))
}

func getAccountIdFromLNURLWithdraw(r *http.Request) (string, error) {
	qs := r.URL.Query()

	// get account id from session
	if session := qs.Get("session"); session != "" {
		acct, err := rds.Get("auth-session:" + session).Result()
		if err == nil {
			return acct, nil
		} else {
			log.Error().Err(err).Str("session", session).
				Msg("failed to get session from redis on withdraw")
			return "", errors.New("lnurl session " + session + " has expired.")

		}
	}

	if acct := qs.Get("acct"); acct != "" {
		if hmacAccount(acct) == qs.Get("hmac") {
			return acct, nil
		} else {
			log.Error().Err(err).Str("hmac", qs.Get("hmac")).Str("accountId", acct).
				Msg("hmacAccount doesn't match supplied hmac param")
			return "", errors.New("Invalid reusable lnurl-withdraw.")
		}
	}

	log.Error().Err(err).Msg("lnurl-withdraw with no reference to any account")
	return "", errors.New("Unexpected incomplete lnurl-withdraw.")
}

func balanceWithReserve(balance int64) int64 {
	return int64(float64(balance) * 0.992)
}
