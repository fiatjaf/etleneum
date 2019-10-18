package main

import (
	"context"
	"database/sql"
	"strings"

	"github.com/fiatjaf/etleneum/types"
	"github.com/tidwall/gjson"
	"gopkg.in/antage/eventsource.v1"
)

func handleInvoicePaid(res gjson.Result) {
	index := res.Get("pay_index").Int()
	rds.Set("lastinvoiceindex", index, 0)

	label := res.Get("label").String()
	msats := res.Get("msatoshi_received").Int()

	log.Info().Str("label", label).Int64("msats", msats).
		Str("desc", res.Get("description").String()).
		Msg("got payment")

	go handlePaymentReceived(label, msats)
}

func handlePaymentReceived(label string, msats int64) {
	log.Debug().Str("label", label).Int64("msats", msats).Msg("handling payment")

	switch {
	case strings.HasPrefix(label, s.ServiceId+"."):
		// a contract or call invoice was paid
		parts := strings.Split(label, ".")
		switch len(parts) {
		case 2:
			// start the contract
			contractId := parts[1]
			logger := log.With().Str("ctid", contractId).Logger()

			ct, err := contractFromRedis(contractId)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to fetch contract from redis to activate")
				if ies, ok := contractstreams.Get(contractId); ok {
					ies.(eventsource.EventSource).SendEventMessage(`{"id": "`+contractId+`", "error": "`+err.Error()+`", "kind": "internal"}`, "contract-error", "")
				}
				return
			}

			txn, err := pg.BeginTxx(context.TODO(),
				&sql.TxOptions{Isolation: sql.LevelSerializable})
			if err != nil {
				logger.Warn().Err(err).Msg("transaction start failed")
				dispatchContractEvent(contractId, ctevent{contractId, "", err.Error(), "internal"}, "contract-error")
				return
			}
			defer txn.Rollback()

			// create initial contract
			_, err = txn.Exec(`
INSERT INTO contracts (id, name, readme, code, state)
VALUES ($1, $2, $3, $4, '{}')
    `, ct.Id, ct.Name, ct.Readme, ct.Code)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to save contract on database")
				dispatchContractEvent(contractId, ctevent{contractId, "", err.Error(), "internal"}, "contract-error")
				return
			}

			// instantiate call
			call := &types.Call{
				ContractId: ct.Id,
				Id:         ct.Id, // same
				Method:     "__init__",
				Payload:    []byte{},
				Cost:       getContractCost(*ct),
			}

			err = runCall(call, txn)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to run call")
				dispatchContractEvent(contractId, ctevent{contractId, "", err.Error(), "runtime"}, "contract-error")
				return
			}
			dispatchContractEvent(contractId, ctevent{contractId, "", "", ""}, "contract-created")
			logger.Info().Msg("contract is live")

			// saved. delete from redis.
			rds.Del("contract:" + contractId)
		case 3:
			// run the call
			contractId := parts[1]
			callId := parts[2]
			logger := log.With().Str("ctid", contractId).Str("callid", callId).Logger()

			call, err := callFromRedis(callId)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to fetch call from redis")
				dispatchContractEvent(contractId, ctevent{callId, call.ContractId, err.Error(), "internal"}, "call-error")
				return
			}

			txn, err := pg.BeginTxx(context.TODO(),
				&sql.TxOptions{Isolation: sql.LevelSerializable})
			if err != nil {
				logger.Warn().Err(err).Msg("transaction start failed")
				dispatchContractEvent(contractId, ctevent{callId, call.ContractId, err.Error(), "internal"}, "call-error")
				return
			}
			defer txn.Rollback()

			logger.Info().Interface("call", call).Msg("call being made")
			err = runCall(call, txn)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to run call")
				dispatchContractEvent(contractId, ctevent{callId, call.ContractId, err.Error(), "runtime"}, "call-error")
				return
			}

			dispatchContractEvent(contractId, ctevent{callId, call.ContractId, "", ""}, "call-made")

			// saved. delete from redis.
			rds.Del("call:" + call.Id)
		}
	default:
		// for now we won't handle this here.
		log.Debug().Str("label", label).Msg("not handling payment.")
	}
}
