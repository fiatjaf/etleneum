package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

func startQueue() {
	go func() {
		for {
			bolt11, contractId, callId := getNext()
			var res gjson.Result
			res, err = ln.CallWithCustomTimeout(time.Second*10, "pay", bolt11)
			log.Debug().Err(err).Str("res", res.String()).
				Str("bolt11", bolt11).
				Str("callid", callId).
				Str("ctid", contractId).
				Msg("payment from contract call")

			blob := encodePendingPayment(bolt11, contractId, callId)
			if err != nil {
				err = rds.SMove("processing-payments", "failed-payments", blob).Err()
				if err != nil {
					log.Error().Err(err).Str("blob", blob).
						Msg("error moving from processing-payments to failed-payments")
				}
			} else {
				err = rds.SRem("processing-payments", blob).Err()
				if err != nil {
					log.Error().Err(err).Str("blob", blob).
						Msg("error moving from processing-payments to failed-payments")
				}
			}
		}
	}()
}

func getNext() (string, string, string) {
	var err error
	log.Print("getting next")

	if rds.SCard("processing-payments").Val() > 0 {
		log.Print("SCARD ", rds.SCard("processing-payments").Val())
		// some payment was pending in this queue, perform it first
		next, err := rds.SRandMember("processing-payments").Result()
		if err != nil {
			log.Error().Err(err).Msg("failed to get directly from processing-payments")
		} else if next != "" {
			return decodePendingPayment(next)
		}
	}

	res, err := rds.BRPop(time.Minute*30, "pending-payments").Result()
	if err != nil {
		return getNext()
	}

	log.Print("GOT BRPOP ", res)

	if len(res) == 0 {
		return getNext()
	}

	next := res[1]
	if next == "" {
		return getNext()
	}

	rds.SAdd("processing-payments", next)

	return decodePendingPayment(next)
}

func encodePendingPayment(bolt11, contractId, callId string) string {
	return fmt.Sprintf("%s.%s.%s", bolt11, contractId, callId)
}

func decodePendingPayment(blob string) (bolt11, contractId, callId string) {
	p := strings.Split(blob, ".")
	if len(p) != 3 {
		log.Fatal().Str("blob", blob).Msg("wrong pending-payment blob")
		return
	}

	bolt11 = p[0]
	contractId = p[1]
	callId = p[2]
	return
}

func queuePayment(bolt11, contractId, callId string) error {
	err := rds.LPush("pending-payments", encodePendingPayment(bolt11, contractId, callId)).Err()
	if err != nil {
		return err
	}
	return nil
}
