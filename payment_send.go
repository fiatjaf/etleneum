package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/tidwall/gjson"
)

func tryPayment(bolt11 string, callId string) {
	// get payment info
	inv, err := ln.Call("decodepay", bolt11)
	if err != nil {
		log.Error().Err(err).Str("bolt11", bolt11).Msg("failed to decode bolt11, will attempt to pay anyway")
	}

	// insert pending outgoing payment on db
	_, err = pg.Exec(`
INSERT INTO outgoing_payments (call_id, bolt11, payee, hash, msatoshi, pending, failed)
VALUES ($1, $2, $3, $4, $5, true, false)
    `, callId, bolt11, inv.Get("payee").String(), inv.Get("payement_hash").String(), inv.Get("msatoshi").Int())
	if err != nil {
		log.Error().Err(err).Str("bolt11", bolt11).Msg("failed to save outgoing payment")
		return
	}

	go actuallyPerformPayment(bolt11)
}

func retryPayment(w http.ResponseWriter, r *http.Request) {
	oldbolt11 := mux.Vars(r)["bolt11"]
	newbolt11 := oldbolt11 // default to the same

	b, err := ioutil.ReadAll(r.Body)
	if err == nil {
		newbolt11 = gjson.GetBytes(b, "invoice").String()
	}

	logger := log.With().Str("o", oldbolt11).Str("n", newbolt11).Logger()

	// check the two amounts
	o, _ := ln.Call("decodepay", oldbolt11)
	n, _ := ln.Call("decodepay", newbolt11)
	if n.Get("msatoshi").Int() != o.Get("msatoshi").Int() {
		logger.Debug().
			Int64("o-msats", o.Get("msatoshi").Int()).Int64("n-msats", n.Get("msatoshi").Int()).
			Msg("retry with invalid amount")
		jsonError(w, "retry with invalid amount", 403)
		return
	}

	// when retrying it is sufficient to know the previous bolt11 as that should have been
	// sent in a hidden field (like {_invoice: 'lnbc...'}) thus only the payee should
	// know the full invoice.

	// delete the old and queue the new
	var ok bool
	err = pg.Get(&ok, `
WITH upd AS (
  UPDATE outgoing_payments
  SET bolt11 = $2, pending = true, failed = false
  WHERE bolt11 = $1 AND pending = false AND failed = true
  RETURNING true
)
SELECT EXISTS (SELECT true FROM upd)
    `, oldbolt11, newbolt11)
	if err != nil {
		if err == sql.ErrNoRows {
			logger.Warn().Str("bolt11", oldbolt11).Msg("retry with payment still pending or not failed")
			jsonError(w, "please wait until the payment has failed completely", 409)
			return
		}

		logger.Error().Err(err).Str("bolt11", oldbolt11).Msg("retry database error")
		jsonError(w, "retry database error, please report", 403)
		return
	}

	// ok, now that we have a new pending bolt11 in the database, let's attempt the payment
	go actuallyPerformPayment(newbolt11)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true})
}

func actuallyPerformPayment(bolt11 string) {
	success, pay, _, err := ln.PayAndWaitUntilResolution(
		bolt11,
		map[string]interface{}{
			"riskfactor":    3,
			"maxfeepercent": 0.5,
			"exemptfee":     1100,
			"retry_for":     s.PaymentRetrySeconds,
		},
	)

	log.Debug().Err(err).Str("bolt11", bolt11).Bool("success", success).Msg("payment retry result")
	if err != nil {
		// something is wrong, maybe we lost connectivity with the lightning node
		// we cannot be sure the payment went through or not
		log.Warn().Err(err).Str("bolt11", bolt11).
			Msg("unexpected error when sending payment, it will be pending forever")
		return
	}

	// we have a definitive answer, can we update the database
	_, err = pg.Exec(`
UPDATE outgoing_payments
SET pending = false,
    failed = $2,
    preimage = $3,
    fee = $4
WHERE bolt11 = $1`,
		bolt11,
		!success,
		pay.Get("payment_preimage").String(),
		pay.Get("msatoshi_sent").Int()-pay.Get("msatoshi").Int(),
	)
	if err != nil {
		log.Error().Err(err).Bool("success", success).Str("bolt11", bolt11).
			Msg("failed to update outgoing payment status")
	}
}
