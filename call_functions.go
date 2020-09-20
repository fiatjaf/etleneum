package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fiatjaf/etleneum/runlua"
	"github.com/fiatjaf/etleneum/types"
	"github.com/jmoiron/sqlx"
	"github.com/lucsky/cuid"
	"github.com/yudai/gojsondiff"
)

func getCallCosts(c types.Call, isLnurl bool) int64 {
	cost := s.FixedCallCostSatoshis * 1000 // a fixed cost of 1 satoshi by default

	if !isLnurl {
		chars := int64(len(string(c.Payload)))
		cost += 10 * chars // 50 msatoshi for each character in the payload
	}

	return cost
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
	callerType := "account"
	if call.Caller != "" && call.Caller[0] == 'c' {
		callerType = "contract"
	}

	_, err = txn.Exec(fmt.Sprintf(`
INSERT INTO calls (id, contract_id, method, payload, cost, msatoshi, caller_%s)
VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, callerType),
		call.Id, call.ContractId,
		call.Method, call.Payload, call.Cost, call.Msatoshi,
		caller)
	if err != nil {
		log.Warn().Err(err).Str("callid", call.Id).Msg("database error")
		return
	}

	// actually run the call
	dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, call.Method, "", "start"}, "call-run-event")
	newStateO, err := runlua.RunCall(
		log,
		&callPrinter{call.ContractId, call.Id, call.Method},
		func(r *http.Request) (*http.Response, error) { return http.DefaultClient.Do(r) },

		// get external contract
		func(contractId string) (state interface{}, funds int64, err error) {
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

		// call external method
		func(externalContractId string, method string, payload interface{}, msatoshi int64) (err error) {
			jpayload, _ := json.Marshal(payload)

			// build the call
			externalCall := &types.Call{
				ContractId: externalContractId,
				Id:         "r" + cuid.Slug(), // a normal new call id
				Method:     method,
				Payload:    jpayload,
				Msatoshi:   msatoshi,
				Cost:       1000, // only the fixed cost, the other costs are included
				Caller:     call.ContractId,
			}

			// pay for the call (by extracting the fixed cost from call satoshis)
			// this external call will already have its cost saved by runCall()
			_, err = txn.Exec(`
UPDATE calls AS c SET msatoshi = c.msatoshi - $2 WHERE id = $1
            `, call.Id, externalCall.Cost)
			if err != nil {
				log.Error().Err(err).Msg("external call cost update failed")
				return
			}

			// transfer funds from current contract to the external contract
			if externalCall.Msatoshi > 0 {
				_, err = txn.Exec(`
INSERT INTO internal_transfers
  (call_id, msatoshi, from_contract, to_contract)
VALUES ($1, $2, $3, $4)
            `, call.Id, externalCall.Msatoshi, ct.Id, externalCall.ContractId)
				if err != nil {
					log.Error().Err(err).Msg("external call transfer failed")
					return
				}
			}

			// then run
			err = runCall(externalCall, txn)
			if err != nil {
				return err
			}

			return nil
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

			dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, call.Method, fmt.Sprintf("contract.send(%s, %d)", target, msat), "function"}, "call-run-event")
			return msat, nil
		},

		// get account balance
		func() (userBalance int, err error) {
			if call.Caller == "" {
				return 0, errors.New("no account")
			}
			err = txn.Get(&userBalance, "SELECT balance($1)", call.Caller)
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
			err = txn.Get(&balance, "SELECT balance($1)", ct.Id)
			if err != nil {
				return
			}
			if balance < 0 {
				return 0, fmt.Errorf("insufficient account balance, needed %d msat more", -balance)
			}

			dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, call.Method, fmt.Sprintf("account.send(%s, %d)", target, msat), "function"}, "call-run-event")
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

	// calculate and save state diff
	differ := gojsondiff.New()
	diff, err := differ.Compare(ct.State, newState)
	if err == nil {
		var adiff []string
		for _, idelta := range diff.Deltas() {
			adiff = append(adiff, diffDeltaOneliner("", idelta)...)
		}
		tdiff := strings.Join(adiff, "\n")
		_, err = txn.Exec(`
UPDATE calls SET diff = $2
WHERE id = $1
    `, call.Id, tdiff)
		if err != nil {
			log.Warn().Err(err).Str("callid", call.Id).
				Str("diff", tdiff).
				Msg("database error")
			return
		}
	} else {
		log.Warn().Err(err).Str("callid", call.Id).Msg("error calculating diff")
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

	// ok, all is good
	log.Info().Str("callid", call.Id).Msg("call done")
	return
}
