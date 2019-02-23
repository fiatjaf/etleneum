package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
)

type Result struct {
	Ok    bool        `json:"ok"`
	Value interface{} `json:"value"`
	Error string      `json:"error,omitempty"`
}

type Contract struct {
	Id           string         `db:"id" json:"id"` // used in the invoice label
	Code         string         `db:"code" json:"code"`
	Name         string         `db:"name" json:"name"`
	Readme       string         `db:"readme" json:"readme"`
	State        types.JSONText `db:"state" json:"state"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
	StorageCosts int            `db:"storage_costs" json:"storage_costs"` // sum of all daily storage costs, in msats
	Refilled     int            `db:"refilled" json:"refilled"`           // msats refilled without use of a normal call

	Funds       int    `db:"funds" json:"funds"` // contract balance in msats
	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid *bool  `db:"-" json:"invoice_paid,omitempty"`
}

func contractFromRedis(ctid string) (ct *Contract, err error) {
	var jct []byte
	ct = &Contract{}

	jct, err = rds.Get("contract:" + ctid).Bytes()
	if err != nil {
		return
	}

	err = json.Unmarshal(jct, ct)
	if err != nil {
		return
	}

	return
}

func (c Contract) getCost() int {
	return 53 * len(c.Code)
}

func (c *Contract) getInvoice() error {
	label := s.ServiceId + "." + c.Id
	desc := s.ServiceId + " __init__ [" + c.Id + "]"
	msats := c.getCost() + 1000*s.InitialContractFillSatoshis
	bolt11, paid, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	c.InvoicePaid = &paid
	return err
}

func (ct Contract) saveOnRedis() (jct []byte, err error) {
	jct, err = json.Marshal(ct)
	if err != nil {
		return
	}

	err = rds.Set("contract:"+ct.Id, jct, time.Hour*20).Err()
	if err != nil {
		return
	}

	return
}

type Call struct {
	Id         string         `db:"id" json:"id"` // used in the invoice label
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Satoshis   int            `db:"satoshis" json:"satoshis"` // sats to be added to the contract
	Cost       int            `db:"cost" json:"cost"`         // msats to be paid to the platform
	Paid       int            `db:"paid" json:"paid"`         // msats sum of payments done by this contract

	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid *bool  `db:"-" json:"invoice_paid,omitempty"`
}

func (c *Call) calcCosts() {
	c.Cost = 1000
	c.Cost += int(float64(len(c.Payload)) * 1.5)
}

func (c *Call) getInvoice() error {
	label := s.ServiceId + "." + c.ContractId + "." + c.Id
	desc := s.ServiceId + " " + c.Method + " [" + c.ContractId + "][" + c.Id + "]"
	msats := c.Cost + 1000*c.Satoshis
	bolt11, paid, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	c.InvoicePaid = &paid
	return err
}

func callFromRedis(callid string) (call *Call, err error) {
	var jcall []byte
	call = &Call{}

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

func (call Call) saveOnRedis() (jcall []byte, err error) {
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

func (call Call) runCall(txn *sqlx.Tx) (ret interface{}, err error) {
	// get contract data
	var ct Contract
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
	newState, totalPaid, paymentsPending, returnedValue, err := runLua(ct, call)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to run call")
		return
	}
	ret = returnedValue

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
