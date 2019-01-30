package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/imdario/mergo"
	"github.com/jmoiron/sqlx/types"
	"github.com/lucsky/cuid"
)

func listContracts(w http.ResponseWriter, r *http.Request) {
	var contracts []Contract
	err = pg.Select(&contracts, `
SELECT id, name, readme FROM (
  SELECT id, name, readme, created_at,
    (SELECT max(time) FROM calls WHERE contract_id = c.id) AS lastcalltime
  FROM contracts AS c
) AS x
ORDER BY lastcalltime DESC, created_at DESC
`)
	if err == sql.ErrNoRows {
		contracts = make([]Contract, 0)
	} else if err != nil {
		log.Warn().Err(err).Msg("failed to fetch contracts")
		http.Error(w, "", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contracts)
}

func prepareContract(w http.ResponseWriter, r *http.Request) {
	// making a contract only saves it temporarily.
	// the contract can be inspected only by its creator.
	// once the creator knows everything is right, he can call init.
	ct := &Contract{}
	err := json.NewDecoder(r.Body).Decode(ct)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse contract json.")
		http.Error(w, "", 400)
		return
	}
	ct.Id = cuid.Slug()

	err = ct.getInvoice()
	if err != nil {
		log.Warn().Err(err).Msg("failed to make invoice.")
		http.Error(w, "", 500)
		return
	}

	jct, err := ct.saveOnRedis()
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		http.Error(w, "", 500)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jct)
}

func getContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct := &Contract{}
	err = pg.Get(ct, `
SELECT *,
  (SELECT sum(satoshis - paid) FROM calls WHERE contract_id = $1) AS funds
FROM contracts
WHERE id = $1`,
		ctid)
	if err == sql.ErrNoRows {
		// couldn't find on database, maybe it's a temporary contract?
		ct, err = contractFromRedis(ctid)
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to fetch fetch contract from redis")
			http.Error(w, "", 404)
			return
		}
		err = ct.getInvoice()
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to get/make invoice")
			http.Error(w, "", 500)
			return
		}
	} else if err != nil {
		// it's a database error
		log.Warn().Err(err).Str("ctid", ctid).Msg("database error fetching contract")
		http.Error(w, "", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ct)
}

func updateContract(w http.ResponseWriter, r *http.Request) {
	// updates a contract before it is initiated
	ctid := mux.Vars(r)["ctid"]

	var upd = Contract{}
	var curr = &Contract{}

	// parse update
	err := json.NewDecoder(r.Body).Decode(&upd)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse contract update json.")
		http.Error(w, "", 400)
		return
	}

	// get current
	curr, err = contractFromRedis(ctid)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get contract from redis.")
		http.Error(w, "", 404)
		return
	}

	// update current
	mergo.Merge(&curr, upd)

	// update invoice
	err = curr.getInvoice()
	if err != nil {
		log.Warn().Err(err).Msg("failed to make invoice.")
		http.Error(w, "", 500)
		return
	}

	// save on redis
	jcurr, err := curr.saveOnRedis()
	if err != nil {
		log.Warn().Err(err).Interface("ct", curr).Msg("failed to save to redis")
		http.Error(w, "", 500)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jcurr)
}

func makeContract(w http.ResponseWriter, r *http.Request) {
	// init is a special call that enables a contract.
	// it has a fixed cost and its payload is the initial state of the contract.
	ctid := mux.Vars(r)["ctid"]
	costsatoshis := s.InitCostSatoshis

	ct, err := contractFromRedis(ctid)
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).
			Msg("failed to fetch contract from redis")
		http.Error(w, "", 404)
		return
	}

	err = checkPayment(s.ServiceId+"."+ctid, costsatoshis)
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).Msg("payment check failed")
		http.Error(w, "", 500)
		return
	}

	// request payload should be the initial state
	bpayload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).
			Msg("failed to read init payload")
		http.Error(w, "", 400)
		return
	}
	payload := string(bpayload)

	_, err = pg.Exec(`
WITH contract AS (
  INSERT INTO contracts (id, name, readme, code, state)
  VALUES ($1, $2, $3, $4, $5)
  RETURNING id
)
INSERT INTO calls (id, contract_id, method, payload, cost, satoshis)
VALUES ($6, (SELECT id FROM contract), '__init__',  $7, $8, 0)
    `, ct.Id, ct.Name, ct.Readme, ct.Code, payload,
		cuid.Slug(), payload, costsatoshis)
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).
			Msg("failed to save contract on database")
		http.Error(w, "", 500)
		return
	}

	// saved. delete from redis.
	rds.Del("contract:" + ctid)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok": true}`))
}

func listCalls(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	var calls []Call
	err = pg.Select(&calls, `
SELECT *
FROM calls
WHERE contract_id = $1
ORDER BY time DESC
        `, ctid)
	if err == sql.ErrNoRows {
		calls = make([]Call, 0)
	} else if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).Msg("failed to fetch calls")
		http.Error(w, "", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(calls)
}

func prepareCall(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	call := &Call{}
	err := json.NewDecoder(r.Body).Decode(call)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse call json.")
		http.Error(w, "", 400)
		return
	}
	call.ContractId = ctid
	call.Id = cuid.Slug()

	call.calcCosts()
	err = call.getInvoice()
	if err != nil {
		log.Warn().Err(err).Msg("failed to make invoice.")
		http.Error(w, "", 500)
		return
	}

	jcall, err := json.Marshal(call)
	if err != nil {
		log.Warn().Err(err).Interface("call", call).Msg("failed to marshal call")
		http.Error(w, "", 500)
		return
	}

	err = rds.Set("call:"+call.Id, jcall, time.Hour*30).Err()
	if err != nil {
		log.Warn().Err(err).Interface("call", call).Msg("failed to save to redis")
		http.Error(w, "", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jcall)
}

func getCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]

	call := &Call{}
	err = pg.Get(call, `
SELECT * FROM calls WHERE id = $1
    `, callid)
	if err == sql.ErrNoRows {
		bcall, err := rds.Get("call:" + callid).Bytes()
		if err != nil || len(bcall) == 0 {
			log.Warn().Err(err).Str("callid", callid).
				Msg("failed to fetch call from redis")
			http.Error(w, "", 404)
			return
		}

		// found on redis
		err = json.Unmarshal(bcall, &call)
		if err != nil {
			log.Warn().Err(err).Str("callid", callid).Str("c", string(bcall)).
				Msg("failed to decode from redis")
			http.Error(w, "", 500)
			return
		}
	} else if err != nil {
		// it's a database error
		log.Warn().Err(err).Str("callid", callid).Msg("database error fetching call")
		http.Error(w, "", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(call)
}

func makeCall(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]

	jcall, err := rds.Get("call:" + callid).Bytes()
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).
			Msg("failed to fetch temporary call for making it")
		http.Error(w, "", 404)
		return
	}

	call := &Call{}
	err = json.Unmarshal(jcall, call)
	if err != nil {
		log.Warn().Err(err).Str("call", string(jcall)).
			Msg("failed to unmarshal temporary call")
		http.Error(w, "", 500)
		return
	}

	log.Info().Interface("call", call).Msg("call being made")

	label := s.ServiceId + "." + call.ContractId + "." + callid
	err = checkPayment(label, call.Cost+call.Satoshis*1000)
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("payment check failed")
		http.Error(w, "", 500)
		return
	}

	// proceed to run the call
	txn, err := pg.BeginTxx(context.TODO(),
		&sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("transaction start failed")
		http.Error(w, "", 500)
		return
	}
	defer txn.Rollback()

	// get contract data
	var ct Contract
	err = txn.Get(&ct, `
SELECT *,
  (SELECT sum(satoshis - paid) FROM calls WHERE contract_id = $1) AS funds
FROM contracts
WHERE id = $1`, call.ContractId)
	if err != nil {
		log.Warn().Err(err).Str("ctid", call.ContractId).Str("callid", callid).
			Msg("failed to get contract data")
		http.Error(w, "", 500)
		return
	}

	// actually run the call
	newState, totalPaid, paymentsPending, returnedValue, err := runLua(ct, *call)
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("failed to run call")
		http.Error(w, "", 500)
		return
	}

	// save new state
	_, err = txn.Exec(`
UPDATE contracts SET state = $2
WHERE id = $1
    `, call.ContractId, newState)
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("database error")
		http.Error(w, "", 500)
		return
	}

	// save call (including all the transactions, even though they weren't paid yet)
	_, err = txn.Exec(`
INSERT INTO calls (id, contract_id, method, payload, cost, satoshis, paid)
VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, call.Id, call.ContractId,
		call.Method, call.Payload, call.Cost, call.Satoshis, totalPaid)
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("database error")
		http.Error(w, "", 500)
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
		log.Warn().Err(err).Str("callid", callid).Msg("database error")
		http.Error(w, "", 500)
		return
	}

	if contractFunds < 0 {
		log.Warn().Err(err).Str("callid", callid).Msg("contract out of funds")
		http.Error(w, "", 500)
		return
	}

	// ok, all is good, commit and proceed.
	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("failed to commit call")
		http.Error(w, "", 500)
		return
	}

	// delete from redis to prevent double-calls
	rds.Del("call:" + callid)

	log.Info().Str("callid", callid).Interface("payments", paymentsPending).
		Msg("call done")

	// everything is saved and well alright.
	// do the payments in a separate goroutine:
	go func(callId string, previousState types.JSONText, paymentsPending []string) {
		dirty := false
		stillpending := make([]string, 0, len(paymentsPending))

		for _, bolt11 := range paymentsPending {
			res, err := ln.CallWithCustomTimeout("pay", time.Second*10, bolt11)
			log.Debug().Err(err).Str("res", res.String()).
				Str("callid", callid).
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"value": returnedValue,
	})
}
