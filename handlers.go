package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

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

	jct, err := json.Marshal(ct)
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to marshal contract")
		http.Error(w, "", 500)
		return
	}

	err = rds.Set("contract:"+ct.Id, jct, time.Hour*30).Err()
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		http.Error(w, "", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jct)
}

func getContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct := &Contract{}
	err = pg.Get(ct, `SELECT * FROM contracts WHERE id = $1`, ctid)
	if err == sql.ErrNoRows {
		// couldn't find on database, maybe it's a temporary contract?
		jct, err := rds.Get("contract:" + ctid).Bytes()
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to fetch contract")
			http.Error(w, "", 404)
			return
		}
		err = json.Unmarshal(jct, ct)
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).Str("b", string(jct)).
				Msg("failed to fetch unmarshal contract from redis")
			http.Error(w, "", 500)
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

func makeContract(w http.ResponseWriter, r *http.Request) {
	// init is a special call that enables a contract.
	// it has a fixed cost and its payload is the initial state of the contract.
	ctid := mux.Vars(r)["ctid"]
	costsatoshis := s.InitCostSatoshis

	jct, err := rds.Get("contract:" + ctid).Bytes()
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).
			Msg("failed to fetch temporary contract for init")
		http.Error(w, "", 404)
		return
	}

	ct := &Contract{}
	err = json.Unmarshal(jct, ct)
	if err != nil {
		log.Warn().Err(err).Str("contract", string(jct)).
			Msg("failed to unmarshal temporary contract")
		http.Error(w, "", 500)
		return
	}

	hash, err := checkPayment(s.ServiceId+"."+ctid, costsatoshis)
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
INSERT INTO calls (hash, id, contract_id, method, payload, cost, satoshis)
VALUES ($6, $7, (SELECT id FROM contract), '__init__', $8, $9, 0)
    `, ct.Id, ct.Name, ct.Readme, ct.Code, payload,
		hash, cuid.Slug(), payload, costsatoshis)
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
SELECT * FROM calls WHERE contract_id = $1
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

	label := s.ServiceId + "." + call.ContractId + "." + callid
	call.Hash, err = checkPayment(label, call.Cost+call.Satoshis*1000)
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
	err = txn.Get(&ct, `SELECT * FROM contracts WHERE id = $1`, call.ContractId)
	if err != nil {
		return
	}

	newState, returnedValue, err := runLua(ct, *call)
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("failed to run call")
		http.Error(w, "", 500)
		return
	}

	_, err = txn.Exec(`
WITH contract AS (
  UPDATE contracts SET state = $2
  WHERE id = $1
  RETURNING id
)
INSERT INTO calls (hash, id, contract_id, method, payload, cost, satoshis)
VALUES ($3, $4, (SELECT id FROM contract), $5, $6, $7, $8)
    `, call.ContractId, newState,
		call.Hash, call.Id, call.Method, call.Payload, call.Cost, call.Satoshis,
	)
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).
			Msg("failed to save call on database")
		http.Error(w, "", 500)
		return
	}

	err = txn.Commit()
	if err != nil {
		log.Warn().Err(err).Str("callid", callid).Msg("failed to commit call")
		http.Error(w, "", 500)
		return
	}

	// saved. delete from redis.
	rds.Del("call:" + callid)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"value": returnedValue,
	})
}
