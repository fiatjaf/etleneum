package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fiatjaf/etleneum/data"
	"github.com/fiatjaf/go-lnurl"
	"gopkg.in/antage/eventsource.v1"
)

func lnurlSession(w http.ResponseWriter, r *http.Request) {
	var es eventsource.EventSource
	session := r.URL.Query().Get("session")

	if session == "" {
		session = lnurl.RandomK1()
	} else {
		// check session validity as k1
		b, err := hex.DecodeString(session)
		if err != nil || len(b) != 32 {
			session = lnurl.RandomK1()
		} else {
			// finally try to fetch an existing stream
			ies, ok := userstreams.Get(session)
			if ok {
				es = ies.(eventsource.EventSource)
			}
		}
	}

	if es == nil {
		es = eventsource.New(
			&eventsource.Settings{
				Timeout:        5 * time.Second,
				CloseOnTimeout: true,
				IdleTimeout:    1 * time.Minute,
			},
			func(r *http.Request) [][]byte {
				return [][]byte{
					[]byte("X-Accel-Buffering: no"),
					[]byte("Cache-Control: no-cache"),
					[]byte("Content-Type: text/event-stream"),
					[]byte("Connection: keep-alive"),
					[]byte("Access-Control-Allow-Origin: *"),
				}
			},
		)
		userstreams.Set(session, es)
		go func() {
			for {
				time.Sleep(25 * time.Second)
				es.SendEventMessage("", "keepalive", "")
			}
		}()
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		es.SendRetryMessage(3 * time.Second)
	}()

	accountId := rds.Get("auth-session:" + session).Val()
	balance := data.GetAccountBalance(accountId)
	if accountId != "" {
		// we're logged already, so send account information
		go func() {
			time.Sleep(100 * time.Millisecond)
			es.SendEventMessage(`{"account": "`+accountId+`", "balance": `+strconv.FormatInt(balance, 10)+`, "can_withdraw": `+strconv.FormatInt(balanceWithReserve(balance), 10)+`, "secret": "`+getAccountSecret(accountId)+`"}`, "auth", "")
		}()

		// also renew this session
		rds.Expire("auth-session:"+session, time.Hour*24*30)
	}

	// always send lnurls because we need lnurl-withdraw even if we're
	// logged already
	go func() {
		time.Sleep(100 * time.Millisecond)
		auth, _ := lnurl.LNURLEncode(s.ServiceURL + "/lnurl/auth?tag=login&k1=" + session)
		withdraw, _ := lnurl.LNURLEncode(s.ServiceURL + "/lnurl/withdraw?session=" + session)

		es.SendEventMessage(`{"auth": "`+auth+`", "withdraw": "`+withdraw+`"}`, "lnurls", "")
	}()

	es.ServeHTTP(w, r)
}

func lnurlAuth(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	k1 := params.Get("k1")
	sig := params.Get("sig")
	key := params.Get("key")

	if ok, err := lnurl.VerifySignature(k1, sig, key); !ok {
		log.Debug().Err(err).Str("k1", k1).Str("sig", sig).Str("key", key).
			Msg("failed to verify lnurl-auth signature")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("signature verification failed."))
		return
	}

	session := k1
	log.Debug().Str("session", session).Str("pubkey", key).Msg("valid login")

	// there must be a valid auth session (meaning an eventsource client) one otherwise something is wrong
	ies, ok := userstreams.Get(session)
	if !ok {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("there's no browser session to authorize."))
		return
	}

	// assign the account id to this session on redis
	if rds.Set("auth-session:"+session, key, time.Hour*24*30).Err() != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("failed to save session."))
		return
	}

	es := ies.(eventsource.EventSource)

	// notify browser
	es.SendEventMessage(`{"session": "`+k1+`", "account": "`+key+`", "balance": `+strconv.FormatInt(data.GetAccountBalance(key), 10)+`, "secret": "`+getAccountSecret(key)+`"}`, "auth", "")

	json.NewEncoder(w).Encode(lnurl.OkResponse())
}

func refreshBalance(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")

	// get account id from session
	accountId, err := rds.Get("auth-session:" + session).Result()
	if err != nil {
		log.Error().Err(err).Str("session", session).
			Msg("failed to get session from redis on refresh")
		w.WriteHeader(500)
		return
	}

	// get balance
	balance := data.GetAccountBalance(accountId)

	if ies, ok := userstreams.Get(session); ok {
		ies.(eventsource.EventSource).SendEventMessage(`{"account": "`+accountId+`", "balance": `+strconv.FormatInt(balance, 10)+`, "can_withdraw": `+strconv.FormatInt(balanceWithReserve(balance), 10)+`, "secret": "`+getAccountSecret(accountId)+`"}`, "auth", "")
	}

	w.WriteHeader(200)
}

func lnurlWithdraw(w http.ResponseWriter, r *http.Request) {
	accountId, err := getAccountIdFromLNURLWithdraw(r)
	if err != nil {
		json.NewEncoder(w).Encode(err.Error())
		return
	}

	// get balance
	balance := balanceWithReserve(data.GetAccountBalance(accountId))

	if balance < 10000 {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("the minimum withdrawal is 10 sat, your balance is " + strconv.FormatInt(balance, 10) + " msat."))
		return
	}

	json.NewEncoder(w).Encode(lnurl.LNURLWithdrawResponse{
		LNURLResponse: lnurl.LNURLResponse{Status: "OK"},
		Tag:           "withdrawRequest",
		Callback: fmt.Sprintf("%s/lnurl/withdraw/callback?%s",
			s.ServiceURL, r.URL.RawQuery),
		K1:                 hex.EncodeToString(make([]byte, 32)), // we don't care
		MaxWithdrawable:    balance,
		MinWithdrawable:    100000,
		DefaultDescription: fmt.Sprintf("etleneum.com %s balance withdraw", accountId),
		BalanceCheck:       getStaticLNURLWithdraw(accountId),
	})
}

func lnurlWithdrawCallback(w http.ResponseWriter, r *http.Request) {
	// 	accountId, err := getAccountIdFromLNURLWithdraw(r)
	// 	if err != nil {
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse(err.Error()))
	// 		return
	// 	}
	//
	// 	bolt11 := r.URL.Query().Get("pr")
	//
	// 	// start withdrawal transaction
	// 	txn, err := pg.BeginTxx(context.TODO(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	// 	if err != nil {
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse("internal database error."))
	// 		return
	// 	}
	// 	defer txn.Rollback()
	//
	// 	if s.FreeMode {
	// 		json.NewEncoder(w).Encode(lnurl.OkResponse())
	// 		return
	// 	}
	//
	// 	// decode invoice
	// 	inv, err := ln.Call("decodepay", bolt11)
	// 	if err != nil {
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse("failed to decode invoice."))
	// 		return
	// 	}
	// 	amount := inv.Get("msatoshi").Int()
	//
	// 	log.Debug().Str("bolt11", bolt11).Str("account", accountId).Int64("amount", amount).
	// 		Msg("got a withdraw payment request")
	//
	// 	reservefee := int64(float64(amount) * 0.007)
	//
	// 	// add a pending withdrawal
	// 	_, err = txn.Exec(`
	// INSERT INTO withdrawals (account_id, msatoshi, fee_msat, fulfilled, bolt11)
	// VALUES ($1, $2, $3, false, $4)
	//     `, accountId, amount, reservefee, bolt11)
	// 	if err != nil {
	// 		log.Warn().Err(err).Msg("error inserting withdrawal")
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse("database error."))
	// 		return
	// 	}
	//
	// 	// check balance afterwards
	// 	var balance int
	// 	err = txn.Get(&balance, "SELECT balance($1)", accountId)
	// 	if err != nil {
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse("database error."))
	// 		return
	// 	}
	// 	if balance < 0 {
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse("insufficient balance."))
	// 		return
	// 	}
	//
	// 	log.Debug().Int("balance after", balance).Msg("will fulfill")
	//
	// 	err = txn.Commit()
	// 	if err != nil {
	// 		log.Warn().Err(err).Msg("error commiting withdrawal")
	// 		json.NewEncoder(w).Encode(lnurl.ErrorResponse("database error."))
	// 		return
	// 	}
	//
	// 	// actually send the payment
	// 	go func() {
	// 		var (
	// 			listpays gjson.Result
	// 		)
	//
	// 		payresp, err := ln.CallWithCustomTimeout(time.Hour*24*30, "pay",
	// 			map[string]interface{}{
	// 				"bolt11":        bolt11,
	// 				"label":         "etleneum withdraw " + accountId,
	// 				"maxfeepercent": 0.7,
	// 				"exemptfee":     0,
	// 				"retry_for":     20,
	// 			})
	// 		log.Debug().Err(err).Str("resp", payresp.String()).
	// 			Str("account", accountId).Str("bolt11", bolt11).
	// 			Msg("withdraw pay result")
	//
	// 		if _, ok := err.(lightning.ErrorCommand); ok {
	// 			goto failure
	// 		}
	//
	// 		if payresp.Get("status").String() == "complete" {
	// 			// calculate actual fee
	// 			lnfee := payresp.Get("msatoshi_sent").Int() - payresp.Get("msatoshi").Int()
	// 			platformfee := int64(payresp.Get("msatoshi").Float() * 0.001)
	// 			fee := lnfee + platformfee
	//
	// 			// mark as fulfilled
	// 			_, err := pg.Exec(`
	//                 UPDATE withdrawals
	//                 SET fulfilled = true
	//                   , fee_msat = $2
	//                 WHERE bolt11 = $1
	//             `, bolt11, fee)
	// 			if err != nil {
	// 				log.Error().Err(err).Str("accountId", accountId).
	// 					Msg("error marking payment as fulfilled")
	// 			}
	//
	// 			return
	// 		}
	//
	// 		// call listpays to check failure
	// 		listpays, _ = ln.Call("listpays", bolt11)
	// 		if listpays.Get("pays.#").Int() == 1 && listpays.Get("pays.0.status").String() != "failed" {
	// 			// not a failure -- but also not a success
	// 			// we don't know what happened, maybe it's pending, so don't do anything
	// 			log.Debug().Str("bolt11", bolt11).
	// 				Msg("we don't know what happened with this payment")
	//
	// 			// notify browser
	// 			if ies, ok := userstreams.Get(r.URL.Query().Get("session")); ok {
	// 				ies.(eventsource.EventSource).SendEventMessage("We don't know what happened with the payment.", "error", "")
	// 			}
	//
	// 			return
	// 		} else if listpays.Get("pays.#").Int() > 1 {
	// 			// this should not happen
	// 			log.Debug().Str("bolt11", bolt11).Msg("this should not happen")
	// 			return
	// 		}
	//
	// 		// if we reached this point then it's because the payment has failed
	// 	failure:
	// 		// delete attempt since it has undoubtely failed
	// 		_, err = pg.Exec(`DELETE FROM withdrawals WHERE bolt11 = $1`, bolt11)
	// 		if err != nil {
	// 			log.Error().Err(err).Str("accountId", accountId).
	// 				Msg("error deleting withdrawal attempt")
	// 		}
	//
	// 		// notify browser
	// 		if ies, ok := userstreams.Get(r.URL.Query().Get("session")); ok {
	// 			ies.(eventsource.EventSource).SendEventMessage("Payment failed.", "error", "")
	// 		}
	// 	}()
	//
	// 	json.NewEncoder(w).Encode(lnurl.OkResponse())
}

func logout(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	rds.Del("auth-session:" + session)
	userstreams.Remove(session)
	w.WriteHeader(200)
}
