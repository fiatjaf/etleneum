package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/fiatjaf/etleneum/data"
	"github.com/fiatjaf/etleneum/runlua"
)

type CallContext struct {
	VisitedContracts map[string]bool
	Transfers        []data.Transfer
	Funds            map[string]int64
	AccountBalances  map[string]int64
}

func getCallCosts(c data.Call, isLnurl bool) int64 {
	cost := s.FixedCallCostSatoshis * 1000 // a fixed cost of 1 satoshi by default

	if !isLnurl {
		chars := int64(len(string(c.Payload)))
		cost += 10 * chars // 50 msatoshi for each character in the payload
	}

	return cost
}

func callFromRedis(callid string) (call *data.Call, err error) {
	var jcall []byte
	call = &data.Call{}

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

func saveCallOnRedis(call data.Call) (jcall []byte, err error) {
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

func runCallGlobal(call *data.Call, useBalance bool) (err error) {
	// initialize context
	callContext := &CallContext{
		VisitedContracts: make(map[string]bool),
		Funds:            make(map[string]int64),
		AccountBalances:  make(map[string]int64),
	}

	// actually run the call
	err = runCall(call, callContext, useBalance)
	if err != nil {
		return err
	}

	// check balances of contracts and accounts involved
	for key, balance := range callContext.AccountBalances {
		if balance < 0 {
			log.Warn().Err(err).Str("callid", call.Id).Str("account", key).
				Msg("account balance out of funds")
			return errors.New("account balance out of funds")
		}

		// also write this
		if err := data.SaveAccountBalance(key, balance); err != nil {
			return fmt.Errorf("error saving account balance: %w", err)
		}
	}
	for id, funds := range callContext.Funds {
		if funds < 0 {
			log.Warn().Err(err).Str("callid", call.Id).Str("contract", id).
				Msg("contract out of funds")
			return errors.New("contract out of funds")
		}

		// also write this
		if err := data.SaveContractFunds(id, funds); err != nil {
			return fmt.Errorf("error saving contract funds: %w", err)
		}
	}

	if err := data.SaveTransfers(call, callContext.Transfers); err != nil {
		return err
	}

	return nil
}

func runCall(call *data.Call, callContext *CallContext, useBalance bool) (err error) {
	if _, visited := callContext.VisitedContracts[call.ContractId]; visited {
		// can't call a method on the same contract (for now?)
		// (if this ever change see ##callswithsameid)
		return errors.New("can't call a method on the same contract")
	}

	// get contract data
	ct, err := data.GetContract(call.ContractId)
	if err != nil {
		return fmt.Errorf("failed to load contract %s: %w", call.ContractId, err)
	}

	callContext.VisitedContracts[call.ContractId] = true

	// pay for this with the caller's balance?
	if call.Caller != "" && useBalance {
		// burn amount corresponding to the call msatoshi + call cost.
		// we don't transfer to the contract directly
		// because the call already has the msatoshi amount assigned to it and that
		// is already automatically added to the contract balance,
		// so the contract would receive the money twice if we also did a transfer here.
		balance := data.GetAccountBalance(call.Caller)
		if balance < call.Msatoshi+call.Cost {
			log.Warn().Err(err).Msg("account has insufficient funds to execute call")
			dispatchContractEvent(call.ContractId,
				ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi,
					"", "balance"},
				"call-error")
			return errors.New("insufficient account balance")
		}

		callContext.AccountBalances[call.Caller] = balance - (call.Msatoshi + call.Cost)
		callContext.Transfers = append(callContext.Transfers, data.Transfer{
			From:     call.Caller,
			To:       "",
			Msatoshi: call.Cost,
		})
		callContext.Transfers = append(callContext.Transfers, data.Transfer{
			From:     call.Caller,
			To:       call.ContractId,
			Msatoshi: call.Msatoshi,
		})
	} else {
		// take note of the amount sent in this call as a transfer
		callContext.Transfers = append(callContext.Transfers, data.Transfer{
			From:     "",
			To:       call.ContractId,
			Msatoshi: call.Msatoshi,
		})
	}

	callContext.Funds[call.ContractId] = ct.Funds + call.Msatoshi

	// actually run the call
	dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi, "", "start"}, "call-run-event")
	newStateO, err := runlua.RunCall(
		log,
		&callPrinter{call.ContractId, call.Id, call.Method},
		func(r *http.Request) (*http.Response, error) { return http.DefaultClient.Do(r) },

		// get external contract
		func(contractId string) (state interface{}, funds int64, err error) {
			ct, err := data.GetContract(contractId)
			if err != nil {
				return
			}
			err = json.Unmarshal(ct.State, &state)
			if err != nil {
				return
			}
			return state, ct.Funds, nil
		},

		// call external method
		func(externalContractId string, method string, payload interface{}, msatoshi int64) (err error) {
			jpayload, _ := json.Marshal(payload)

			// build the call
			externalCall := &data.Call{
				ContractId: externalContractId,
				Id:         call.Id, // repeat the call id ##callswithsameid
				// (since we won't do two calls with the same id on the same contract)
				Method:   method,
				Payload:  jpayload,
				Msatoshi: msatoshi,
				Cost:     1000, // only the fixed cost, the other costs are included
				Caller:   call.ContractId,
			}

			// pay for the call (by burning msatoshis from the caller contract)
			callContext.Funds[call.ContractId] -= (externalCall.Cost + externalCall.Msatoshi)
			callContext.Transfers = append(callContext.Transfers, data.Transfer{
				From:     call.ContractId,
				To:       externalCall.ContractId,
				Msatoshi: externalCall.Cost + externalCall.Msatoshi,
			})

			// then run
			err = runCall(externalCall, callContext, false)
			if err != nil {
				return err
			}

			return nil
		},

		// get contract balance
		func() (contractFunds int64, err error) {
			ct, err := data.GetContract(ct.Id)
			if err != nil {
				return
			}
			return ct.Funds, nil
		},

		// send from contract
		func(target string, msat int64) (msatoshiSent int64, err error) {
			if len(target) == 0 {
				return 0, errors.New("can't send to blank recipient")
			}

			callContext.Transfers = append(callContext.Transfers, data.Transfer{
				From:     call.ContractId,
				To:       target,
				Msatoshi: msat,
			})
			callContext.Funds[call.ContractId] -= msat

			if target[0] == 'c' {
				// it's a contract
				if current, ok := callContext.Funds[target]; ok {
					callContext.Funds[target] = current + msat
				} else {
					targetContract, err := data.GetContract(target)
					if err != nil {
						return 0, errors.New("contract " + target + " not found")
					}
					callContext.Funds[target] = targetContract.Funds + msat
				}
			} else if target[0] == '0' {
				// it's an account
				if current, ok := callContext.AccountBalances[target]; ok {
					callContext.AccountBalances[target] = current + msat
				} else {
					current := data.GetAccountBalance(target)
					callContext.AccountBalances[target] = current + msat
				}
			} else {
				return 0, errors.New("invalid recipient " + target)
			}

			dispatchContractEvent(call.ContractId, ctevent{call.Id, call.ContractId, call.Method, call.Msatoshi, fmt.Sprintf("contract.send(%s, %d)", target, msat), "function"}, "call-run-event")
			return msat, nil
		},

		// get account balance
		func() (userBalance int64, err error) {
			if call.Caller == "" {
				return 0, errors.New("no account")
			}
			return data.GetAccountBalance(call.Caller), nil
		},

		*ct,
		*call,
	)
	if err != nil {
		return fmt.Errorf("error executing method: %w", err)
	}

	newState, err := json.Marshal(newStateO)
	if err != nil {
		return fmt.Errorf("error marshaling new state: %w", err)
	}

	// write call files
	if err = data.SaveCall(call); err != nil {
		return fmt.Errorf("error saving call data: %w", err)
	}

	if err := data.SaveContractState(call.ContractId, newState); err != nil {
		return fmt.Errorf("error saving contract state: %w", err)
	}

	// ok, all is good
	log.Info().Str("callid", call.Id).Msg("call done")
	return
}
