package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/tidwall/gjson"
	"gopkg.in/redis.v5"
)

const (
	PENDING_QUEUE   = "pending-payments"
	PROCESSING_POOL = "processing-payments"
	FAILED_POOL     = "failed-payments"
)

func startQueue() {
	go func() {
		for {
			bolt11 := getNext()

			logger := log.With().Str("bolt11", bolt11).Logger()
			logger.Debug().Msg("making payment from contract call")

			var res gjson.Result
			res, err := ln.CallWithCustomTimeout(
				time.Second*time.Duration(s.PaymentRetrySeconds)+5,
				"pay", map[string]interface{}{
					"bolt11":        bolt11,
					"riskfactor":    3,
					"maxfeepercent": 0.7,
					"exemptfee":     1100,
					"retry_for":     s.PaymentRetrySeconds,
				})

			if err != nil {
				logger.Warn().Err(err).Str("res", res.String()).Msg("payment failed")

				err = rds.SMove(PROCESSING_POOL, FAILED_POOL, bolt11).Err()
				if err != nil {
					logger.Error().Err(err).Str("bolt11", bolt11).
						Msg("error moving from processing-payments to failed-payments")
				}
			} else {
				err = rds.SRem(PROCESSING_POOL, bolt11).Err()
				if err != nil {
					logger.Error().Err(err).Str("bolt11", bolt11).
						Msg("error moving from processing-payments to failed-payments")
				}

				logger.Warn().Msg("payment succeeded")
			}
		}
	}()
}

func getNext() (bolt11 string) {
	var next string

	err := rds.Watch(func(rtx *redis.Tx) error {
		var err error
		if rds.SCard(PROCESSING_POOL).Val() > 0 {
			// some payment was pending in this queue, perform it first
			next, err = rds.SRandMember(PROCESSING_POOL).Result()
			if err != nil {
				log.Error().Err(err).Msg("failed to get directly from processing-payments")
			} else if next != "" {
				return nil
			}
		}

		res, err := rds.BRPop(time.Minute*30, PENDING_QUEUE).Result()
		if err != nil {
			return err
		}

		if len(res) == 0 {
			return errors.New("queue empty")
		}

		next = res[1]
		if next == "" {
			return errors.New("got blank invoice from queue")
		}

		err = rds.SAdd(PROCESSING_POOL, next).Err()
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return getNext()
	}

	return next
}

func queuePayment(bolt11, contractId, callId string) error {
	err := rds.LPush(PENDING_QUEUE, bolt11).Err()
	if err != nil {
		return err
	}
	return nil
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
	err = rds.Watch(func(rtx *redis.Tx) error {
		if !rtx.SIsMember(FAILED_POOL, oldbolt11).Val() {
			logger.Warn().Err(err).Msg("payment to retry not found")
			jsonError(w, "payment retry not found", 404)
			return errors.New("not found")
		}

		err := rtx.SRem(FAILED_POOL, oldbolt11).Err()
		if err != nil {
			logger.Warn().Err(err).Msg("failed to remove previous payment from queue")
			jsonError(w, "", 500)
			return err
		}

		err = rtx.LPush(PENDING_QUEUE, newbolt11).Err()
		if err != nil {
			logger.Warn().Err(err).Msg("failed to remove previous payment from queue")
			jsonError(w, "", 500)
			return err
		}

		return nil
	})
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true})
}
