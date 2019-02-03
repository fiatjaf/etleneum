package main

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
)

type Contract struct {
	Id        string         `db:"id" json:"id"`
	Code      string         `db:"code" json:"code"`
	Name      string         `db:"name" json:"name"`
	Readme    string         `db:"readme" json:"readme"`
	State     types.JSONText `db:"state" json:"state"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
	Cost      int            `db:"cost" json:"cost"`

	Funds       int    `db:"funds" json:"funds"`
	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid bool   `db:"-" json:"invoice_paid"`
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

func (c *Contract) calcCosts() {
	c.Cost = len(c.Code) * 53
}

func (c *Contract) getInvoice() error {
	label := s.ServiceId + "." + c.Id
	desc := "etleneum __init__ [" + c.Id + "]"
	bolt11, paid, err := getInvoice(label, desc, c.Cost)
	c.Bolt11 = bolt11
	c.InvoicePaid = paid
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

func (call Call) runCall(txn *sqlx.Tx) (ret interface{}, err error) {
	// get contract data
	var ct Contract
	err = txn.Get(&ct, `
SELECT contract.*,
  coalesce(sum(satoshis - paid), 0) AS funds
FROM contracts AS contract
LEFT OUTER JOIN calls AS call ON call.contract_id = contract.id
WHERE contract.id = $1
GROUP BY contract.id`,
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
		log.Warn().Err(err).Str("callid", call.Id).Msg("database error")
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
SELECT sum(satoshis - paid)
FROM calls
WHERE contract_id = $1`,
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
	// do the payments in a separate goroutine:
	go func(callId string, previousState types.JSONText, paymentsPending []string) {
		dirty := false
		stillpending := make([]string, 0, len(paymentsPending))

		for _, bolt11 := range paymentsPending {
			res, err := ln.CallWithCustomTimeout(time.Second*10, "pay", bolt11)
			log.Debug().Err(err).Str("res", res.String()).
				Str("callid", callId).
				Msg("payment from contract")

			if err == nil {
				// at least one payment went through, this whole thing is now dirty
				dirty = true
			} else {
				if dirty == false {
					// if no payment has been made yet, revert this call
					_, err := pg.Exec(`
WITH deleted_call AS (
  DELETE FROM calls WHERE id = $1 
  RETURNING contract_id
)
UPDATE contracts SET state = $2
WHERE id (SELECT contract_id FROM deleted_call)
        `, callId, previousState)
					if err == nil {
						log.Info().Str("callid", callId).
							Str("state", string(previousState)).
							Msg("reverted call")
						return
					} else {
						log.Error().Err(err).Str("callid", callId).
							Str("state", string(previousState)).
							Msg("couldn't revert call after payment failure.")

						// mark all as pending
						stillpending = paymentsPending
						return
					}
				}

				// otherwise the call can't be reverted
				// we'll try to pay again later
				stillpending = append(stillpending, bolt11)
			}
		}

		for _, bolt11 := range stillpending {
			rds.SAdd("pending:"+callId, bolt11)
		}
	}(call.Id, ct.State, paymentsPending)

	return
}

type Call struct {
	Id         string         `db:"id" json:"id"`
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Satoshis   int            `db:"satoshis" json:"satoshis"`
	Cost       int            `db:"cost" json:"cost"`
	Paid       int            `db:"paid" json:"paid"`

	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid bool   `db:"-" json:"invoice_paid"`
}

func (c *Call) calcCosts() {
	c.Cost = 1000
	c.Cost += int(float64(len(c.Payload)) * 1.5)
}

func (c *Call) getInvoice() error {
	label := s.ServiceId + "." + c.ContractId + "." + c.Id
	desc := "etleneum " + c.Method + " [" + c.ContractId + "][" + c.Id + "]"
	msats := c.Cost + 1000*c.Satoshis
	bolt11, paid, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	c.InvoicePaid = paid
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

type Result struct {
	Ok    bool        `json:"ok"`
	Value interface{} `json:"value"`
	Error string      `json:"error,omitempty"`
}
