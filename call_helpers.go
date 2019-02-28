package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/fiatjaf/etleneum/runlua"
	"github.com/fiatjaf/etleneum/types"
	"github.com/jmoiron/sqlx"
	"github.com/tidwall/gjson"
)

func calcCallCosts(c *types.Call) {
	c.Cost = s.FixedCallCostSatoshis * 1000
	c.Cost += int(float64(len(c.Payload)) * 1.5)
}

func getCallInvoice(c *types.Call) error {
	label := s.ServiceId + "." + c.ContractId + "." + c.Id
	desc := s.ServiceId + " " + c.Method + " [" + c.ContractId + "][" + c.Id + "]"
	msats := c.Cost + 1000*c.Satoshis
	bolt11, paid, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	c.InvoicePaid = &paid
	return err
}

func callFromRedis(callid string) (call *types.Call, err error) {
	var jcall []byte
	call = &types.Call{}

	jcall, err = rds.Get("call:" + callid).Bytes()
	if err != nil {
		return
	}

	err = json.Unmarshal(jcall, call)
	if err != nil {
		return
	}

	return
}

func saveCallOnRedis(call types.Call) (jcall []byte, err error) {
	jcall, err = json.Marshal(call)
	if err != nil {
		return
	}

	err = rds.Set("call:"+call.Id, jcall, time.Hour*20).Err()
	if err != nil {
		return
	}

	return
}

func deletePayloadHiddenFields(call *types.Call) error {
	// fields starting with _ are not saved in the contract log
	if len(call.Payload) == 0 {
		return nil
	}

	payload := make(map[string]interface{})
	err := json.Unmarshal([]byte(call.Payload), &payload)
	if err != nil {
		return err
	}

	for k := range payload {
		if strings.HasPrefix(k, "_") {
			delete(payload, k)
		}
	}

	call.Payload, err = json.Marshal(payload)
	return err
}

func runCall(call *types.Call, txn *sqlx.Tx) (ret interface{}, err error) {
	// get contract data
	var ct types.Contract
	err = txn.Get(&ct, `
SELECT *, contracts.funds
FROM contracts
WHERE id = $1`,
		call.ContractId)
	if err != nil {
		log.Warn().Err(err).Str("ctid", call.ContractId).Str("callid", call.Id).
			Msg("failed to get contract data")
		return
	}

	// actually run the call
	bsandboxCode, _ := Asset("static/sandbox.lua")
	sandboxCode := string(bsandboxCode)
	newStateO, totalPaid, paymentsPending, returnedValue, err := runlua.RunCall(
		sandboxCode,
		func(inv string) (gjson.Result, error) { return ln.Call("decodepay", inv) },
		ct,
		*call,
	)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to run call")
		return
	}

	ret = returnedValue
	newState, err := json.Marshal(newStateO)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to marshal new state")
		return
	}

	// save new state
	_, err = txn.Exec(`
UPDATE contracts SET state = $2
WHERE id = $1
    `, call.ContractId, newState)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Str("state", string(newState)).
			Msg("database error")
		return
	}

	err = deletePayloadHiddenFields(call)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Str("payload", string(call.Payload)).
			Msg("failed to delete payload hidden fields")
		return
	}

	// save call (including all the transactions, even though they weren't paid yet)
	_, err = txn.Exec(`
INSERT INTO calls (id, contract_id, method, payload, cost, satoshis, paid)
VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, call.Id, call.ContractId,
		call.Method, call.Payload, call.Cost, call.Satoshis, totalPaid)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("database error")
		return
	}

	// get contract balance (if balance is negative after the call all will fail)
	var contractFunds int
	err = txn.Get(&contractFunds, `
SELECT contracts.funds
FROM contracts WHERE id = $1`,
		call.ContractId)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("database error")
		return
	}

	if contractFunds < 0 {
		log.Warn().Err(err).Str("callid", call.Id).Msg("contract out of funds")
		return
	}

	// ok, all is good, commit and proceed.
	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to commit call")
		return
	}

	// delete from redis to prevent double-calls (will fail in __init__ calls)
	rds.Del("call:" + call.Id)

	log.Info().Str("callid", call.Id).Interface("payments", paymentsPending).
		Msg("call done")

		// everything is saved and well alright.
	// queue the payments (calls can't be reversed)
	for _, bolt11 := range paymentsPending {
		if err := queuePayment(bolt11, call.ContractId, call.Id); err != nil {
			log.Error().Err(err).Str("bolt11", bolt11).
				Str("callid", call.Id).Str("ctid", call.ContractId).
				Msg("failed to queue payment")
		}
	}

	return
}
