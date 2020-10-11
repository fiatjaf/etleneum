package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/lightningd-gjson-rpc/plugin"
)

var continueHTLC = map[string]interface{}{"result": "continue"}
var failHTLC = map[string]interface{}{"result": "fail", "failure_code": 16392}
var failUnknown = map[string]interface{}{"result": "fail", "failure_code": 16399}

func htlc_accepted(p *plugin.Plugin, params plugin.Params) (resp interface{}) {
	amount := params.Get("htlc.amount").String()
	scid := params.Get("onion.short_channel_id").String()
	if scid == "0x0x0" {
		// payment coming to this node, accept it
		return continueHTLC
	}

	hash := params.Get("htlc.payment_hash").String()

	p.Logf("got HTLC. amount=%s short_channel_id=%s hash=%s", amount, scid, hash)
	for rds == nil || pg == nil {
		p.Log("htlc_accepted: waiting until redis and postgres are available.")
		time.Sleep(1 * time.Second)
	}

	msatoshi, err := strconv.ParseInt(amount[:len(amount)-4], 10, 64)
	if err != nil {
		// I don't know what is happening
		p.Logf("error parsing onion.forward_amount: %s - continue", err.Error())
		return continueHTLC
	}

	bscid, err := decodeShortChannelId(scid)
	if err != nil {
		p.Logf("onion.short_channel_id is not in the usual format - continue")
		return continueHTLC
	}

	id, ok := parseShortChannelId(bscid)
	if !ok {
		// it's not an invoice for an etleneum call or contract
		p.Logf("failed to parse onion.short_channel_id - continue")
		return continueHTLC
	}

	// ensure that we can derive the correct preimage for this payment
	preimage := makePreimage(id)
	preimageHex := hex.EncodeToString(preimage)
	derivedHash := sha256.Sum256(preimage)
	derivedHashHex := hex.EncodeToString(derivedHash[:])
	if hash != derivedHashHex {
		p.Logf("we have a preimage %s, but its hash %s didn't match the expected hash %s - fail with incorrect_or_unknown_payment_details", preimageHex, derivedHashHex, hash)
		return failUnknown
	}

	// run the call
	if id[0] == 'c' {
		ok = contractPaymentReceived(id, msatoshi)
	} else if id[0] == 'r' {
		ok = callPaymentReceived(id, msatoshi)
	} else {
		// it's not an invoice for an etleneum call or contract
		p.Logf("parsed id is not an etleneum payment (%s) - continue", id)
		return continueHTLC
	}

	// after the call succeeds, we resolve the payment
	if ok {
		p.Logf("call went ok. we have a preimage: %s - resolve", preimageHex)
		return map[string]interface{}{
			"result":      "resolve",
			"payment_key": preimageHex,
		}
	} else {
		// in case of call execution failure we just fail the payment
		p.Logf("call failed - fail")
		return failHTLC
	}
}

func contractPaymentReceived(contractId string, msatoshi int64) (ok bool) {
	// start the contract
	logger := log.With().Str("ctid", contractId).Logger()

	ct, err := contractFromRedis(contractId)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch contract from redis to activate")
		dispatchContractEvent(contractId,
			ctevent{contractId, "", "", 0, err.Error(), "internal"}, "contract-error")
		return false
	}

	if getContractCost(*ct) > msatoshi {
		return false
	}

	txn, err := pg.BeginTxx(context.TODO(),
		&sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		logger.Warn().Err(err).Msg("transaction start failed")
		dispatchContractEvent(contractId,
			ctevent{contractId, "", "", 0, err.Error(), "internal"}, "contract-error")
		return false
	}
	defer txn.Rollback()

	// create initial contract
	_, err = txn.Exec(`
INSERT INTO contracts (id, name, readme, code, state)
VALUES ($1, $2, $3, $4, '{}')
    `, ct.Id, ct.Name, ct.Readme, ct.Code)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to save contract on database")
		dispatchContractEvent(contractId,
			ctevent{contractId, "", "", 0, err.Error(), "internal"}, "contract-error")
		return false
	}

	// instantiate call (the __init__ special kind)
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
		dispatchContractEvent(contractId,
			ctevent{contractId, "", call.Method, 0, err.Error(), "runtime"}, "contract-error")
		return false
	}

	// commit contract call
	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to commit contract")
		return false
	}

	dispatchContractEvent(contractId,
		ctevent{contractId, "", call.Method, 0, "", ""}, "contract-created")
	logger.Info().Msg("contract is live")

	// saved. delete from redis.
	rds.Del("contract:" + contractId)

	// save contract on github
	saveContractOnGitHub(ct)

	return true
}

func callPaymentReceived(callId string, msatoshi int64) (ok bool) {
	// run the call
	logger := log.With().Str("callid", callId).Logger()

	call, err := callFromRedis(callId)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		return false
	}
	logger = logger.With().Str("ct", call.ContractId).Logger()

	if call.Msatoshi+call.Cost > msatoshi {
		// TODO: this is the place where we should handle MPP payments
		logger.Warn().Int64("got", msatoshi).Int64("needed", call.Msatoshi+call.Cost).
			Msg("insufficient payment amount")
		return false
	}
	// if msatoshi is bigger than needed we take it as a donation

	txn, err := pg.BeginTxx(context.TODO(),
		&sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		logger.Warn().Err(err).Msg("transaction start failed")
		dispatchContractEvent(call.ContractId,
			ctevent{callId, call.ContractId, call.Method, call.Msatoshi, err.Error(), "internal"}, "call-error")
		return false
	}
	defer txn.Rollback()

	logger.Info().Interface("call", call).Msg("call being made")

	// a normal call
	err = runCall(call, txn)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to run call")
		dispatchContractEvent(call.ContractId,
			ctevent{callId, call.ContractId, call.Method, call.Msatoshi, err.Error(), "runtime"}, "call-error")

		return false
	}

	// commit
	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to commit call")
		return false
	}

	dispatchContractEvent(call.ContractId,
		ctevent{callId, call.ContractId, call.Method, call.Msatoshi, "", ""}, "call-made")

	// saved. delete from redis.
	rds.Del("call:" + call.Id)

	return true
}
