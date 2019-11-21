package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/go-lnurl"
)

func listRefunds(w http.ResponseWriter, r *http.Request) {
	refunds := make([]types.Refund, 0)
	err = pg.Select(&refunds, `
WITH nada AS (
  DELETE FROM refunds WHERE fulfilled AND time < now() - '365 days'::interval
)

SELECT `+types.REFUNDFIELDS+`
FROM refunds
WHERE claimed = false
  AND time > now() - '90 days'::interval
ORDER BY time DESC
    `)
	if err == sql.ErrNoRows {
		refunds = make([]types.Refund, 0)
	} else if err != nil {
		log.Warn().Err(err).Msg("failed to list refunds")
		jsonError(w, "failed to list refunds", 500)
		return
	}

	json.NewEncoder(w).Encode(Result{Ok: true, Value: refunds})
}

func lnurlRefund(w http.ResponseWriter, r *http.Request) {
	preimage := r.URL.Query().Get("preimage")
	bpreimage, err := hex.DecodeString(preimage)
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("invalid hex: " + preimage))
		return
	}

	// get refund
	hash := fmt.Sprintf("%x", sha256.Sum256(bpreimage))
	var refund types.Refund
	err = pg.Get(&refund, `
SELECT `+types.REFUNDFIELDS+`
FROM refunds
WHERE payment_hash = $1 AND claimed = false
    `, hash)
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("no refundable calls found for preimage " + preimage))
		return
	}

	json.NewEncoder(w).Encode(lnurl.LNURLWithdrawResponse{
		LNURLResponse:      lnurl.LNURLResponse{Status: "OK"},
		Callback:           fmt.Sprintf("%s/lnurl/refund/callback", s.ServiceURL),
		K1:                 preimage,
		MaxWithdrawable:    int64(refund.Msatoshi),
		MinWithdrawable:    int64(refund.Msatoshi),
		DefaultDescription: fmt.Sprintf("etleneum.com %s refund", hash),
		Tag:                "withdrawRequest",
	})
}

func lnurlRefundCallback(w http.ResponseWriter, r *http.Request) {
	bolt11 := r.URL.Query().Get("pr")
	preimage := r.URL.Query().Get("k1")

	bpreimage, err := hex.DecodeString(preimage)
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("invalid hex: " + preimage))
		return
	}

	// start withdrawal transaction
	txn, err := pg.BeginTxx(context.TODO(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("internal database error."))
		return
	}
	defer txn.Rollback()

	if s.FreeMode {
		json.NewEncoder(w).Encode(lnurl.OkResponse())
		return
	}

	// get refund and set the bolt11 field
	// the claimed field will tell us if we're already trying to refund this
	hash := fmt.Sprintf("%x", sha256.Sum256(bpreimage))
	var refund types.Refund
	err = pg.Get(&refund, `
UPDATE refunds
SET
  claimed = true,
  bolt11 = $2
WHERE payment_hash = $1 AND claimed = false
RETURNING `+types.REFUNDFIELDS+`
    `, hash, bolt11)
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("no refundable calls found for preimage " + preimage))
		return
	}

	// decode invoice
	inv, err := ln.Call("decodepay", bolt11)
	if err != nil {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("failed to decode invoice."))
		return
	}
	amount := inv.Get("msatoshi").Int()

	// check invoice amount
	if amount != int64(refund.Msatoshi) {
		json.NewEncoder(w).Encode(lnurl.ErrorResponse(fmt.Sprintf(
			"invoice amount doesn't match: %d != %d",
			amount, refund.Msatoshi),
		))
		return
	}

	log.Debug().Str("bolt11", bolt11).Str("hash", hash).Int64("amount", amount).
		Msg("got a refund payment request")

	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Msg("error commiting withdrawal")
		json.NewEncoder(w).Encode(lnurl.ErrorResponse("database error."))
		return
	}

	// actually send the payment
	go func() {
		payresp, err := ln.Call("waitpay", bolt11)
		log.Debug().Err(err).Str("resp", payresp.String()).Str("hash", hash).Str("bolt11", bolt11).
			Msg("refund waitpay result")

		if payresp.Get("status").String() == "complete" {
			// mark as fulfilled
			_, err := pg.Exec(`UPDATE refunds SET fulfilled = true WHERE payment_hash = $1`, hash)
			if err != nil {
				log.Error().Err(err).Str("hash", hash).Msg("error marking payment as fulfilled")
			}
		} else {
			// mark as unclaimed -- allow other people to claim it
			_, err := pg.Exec(`UPDATE refunds SET claimed = false WHERE payment_hash = $1`, hash)
			if err != nil {
				log.Error().Err(err).Str("hash", hash).Msg("error marking payment as not claimed")
			}
		}
	}()

	json.NewEncoder(w).Encode(lnurl.OkResponse())
}
