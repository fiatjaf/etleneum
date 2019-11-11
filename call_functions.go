package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/fiatjaf/etleneum/runlua"
	runlua_assets "github.com/fiatjaf/etleneum/runlua/assets"
	"github.com/fiatjaf/etleneum/types"
	"github.com/jmoiron/sqlx"
)

func getCallCosts(c types.Call) int {
	cost := s.FixedCallCostSatoshis * 1000 // a fixed cost of 1 satoshi by default

	chars := len(string(c.Payload))
	cost += 20 * chars // 70 msatoshi for each character in the payload

	if c.Msatoshi > 50000 {
		// to help cover withdraw fees later we charge a percent of the amount of satoshis included
		cost += int(float64(c.Msatoshi) / 100)
	}

	return cost
}

func setCallInvoice(c *types.Call) (label string, err error) {
	label = s.ServiceId + "." + c.ContractId + "." + c.Id
	desc := s.ServiceId + " " + c.Method + " [" + c.ContractId + "][" + c.Id + "]"
	c.Cost = getCallCosts(*c)
	msats := c.Cost + c.Msatoshi
	bolt11, paid, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	c.InvoicePaid = &paid
	return
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

func runCall(call *types.Call, txn *sqlx.Tx) (err error) {
	// get contract data
	var ct types.Contract
	err = txn.Get(&ct, `
SELECT `+types.CONTRACTFIELDS+`, contracts.funds
FROM contracts
WHERE id = $1`,
		call.ContractId)
	if err != nil {
		log.Warn().Err(err).Str("ctid", call.ContractId).Str("callid", call.Id).
			Msg("failed to get contract data")
		return
	}

	// caller can be either an account id or null
	caller := sql.NullString{Valid: call.Caller != "", String: call.Caller}

	// save call data even though we don't know if it will succeed or not (this is a transaction anyway)
	_, err = txn.Exec(`
INSERT INTO calls (id, contract_id, method, payload, cost, msatoshi, caller)
VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, call.Id, call.ContractId, call.Method, call.Payload, call.Cost, call.Msatoshi, caller)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("database error")
		return
	}

	// actually run the call
	bsandboxCode, _ := runlua_assets.Asset("runlua/assets/sandbox.lua")
	sandboxCode := string(bsandboxCode)
	dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, "", "start"}, "call-run-event")
	newStateO, err := runlua.RunCall(
		sandboxCode,
		&callPrinter{call.ContractId, call.Id},
		func(r *http.Request) (*http.Response, error) { return http.DefaultClient.Do(r) },

		// get external contract
		func(contractId string) (state interface{}, funds int, err error) {
			var data types.Contract
			err = txn.Get(&data, "SELECT state, contracts.funds FROM contracts WHERE id = $1", contractId)
			if err != nil {
				return
			}
			err = json.Unmarshal(data.State, &state)
			if err != nil {
				return
			}
			return state, data.Funds, nil
		},

		// get contract balance
		func() (contractFunds int, err error) {
			err = txn.Get(&contractFunds, "SELECT contracts.funds FROM contracts WHERE id = $1", ct.Id)
			return
		},

		// send from contract
		func(target string, msat int) (msatoshiSent int, err error) {
			var totype string
			if len(target) == 0 {
				return 0, errors.New("can't send to blank recipient")
			} else if target[0] == 'c' {
				totype = "contract"
			} else if target[0] == 'a' {
				totype = "account"
			} else {
				return 0, errors.New("invalid recipient " + target)
			}

			_, err = txn.Exec(`
INSERT INTO internal_transfers (call_id, msatoshi, from_contract, to_`+totype+`)
VALUES ($1, $2, $3, $4)
            `, call.Id, msat, ct.Id, target)
			if err != nil {
				return
			}

			var funds int
			err = txn.Get(&funds, "SELECT contracts.funds FROM contracts WHERE id = $1", ct.Id)
			if err != nil {
				return
			}
			if funds < 0 {
				return 0, fmt.Errorf("insufficient contract funds, needed %d msat more", -funds)
			}

			dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, fmt.Sprintf("contract.send(%s, %d)", target, msat), "function"}, "call-run-event")
			return msat, nil
		},

		// get account balance
		func() (userBalance int, err error) {
			if call.Caller == "" {
				return 0, errors.New("no account")
			}
			err = txn.Get(&userBalance, "SELECT accounts.balance FROM accounts WHERE id = $1", call.Caller)
			return
		},

		// send from current account
		func(target string, msat int) (msatoshiSent int, err error) {
			var totype string
			if len(target) == 0 {
				return 0, errors.New("can't send to blank recipient")
			} else if target[0] == 'c' {
				totype = "contract"
			} else if target[0] == 'a' {
				totype = "account"
			} else {
				return 0, errors.New("invalid recipient " + target)
			}
			_, err = txn.Exec(`
INSERT INTO internal_transfers (call_id, msatoshi, from_account, to_`+totype+`)
VALUES ($1, $2, $3, $4)
            `, call.Id, msat, call.Caller, target)
			if err != nil {
				return
			}

			var balance int
			err = txn.Get(&balance, "SELECT accounts.balance FROM accounts WHERE id = $1", ct.Id)
			if err != nil {
				return
			}
			if balance < 0 {
				return 0, fmt.Errorf("insufficient account balance, needed %d msat more", -balance)
			}

			dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, fmt.Sprintf("account.send(%s, %d)", target, msat), "function"}, "call-run-event")
			return msat, nil
		},
		ct,
		*call,
	)
	if err != nil {
		return
	}

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
		err = errors.New("contract out of funds")
		return
	}

	// ok, all is good, commit and proceed.
	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("failed to commit call")
		return
	}

	log.Info().Str("callid", call.Id).Msg("call done")
	return
}
