package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
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
	if err == nil || err == sql.ErrNoRows {
		contracts = make([]Contract, 0)
	} else if err != nil {
		log.Warn().Err(err).Msg("failed to fetch contracts")
		jsonError(w, "", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: contracts})
}

func prepareContract(w http.ResponseWriter, r *http.Request) {
	// making a contract only saves it temporarily.
	// the contract can be inspected only by its creator.
	// once the creator knows everything is right, he can call init.
	ct := &Contract{}
	err := json.NewDecoder(r.Body).Decode(ct)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse contract json.")
		jsonError(w, "", 400)
		return
	}
	ct.Id = cuid.Slug()

	err = ct.getInvoice()
	if err != nil {
		log.Warn().Err(err).Msg("failed to make invoice.")
		jsonError(w, "", 500)
		return
	}

	_, err = ct.saveOnRedis()
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		jsonError(w, "", 500)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: ct})
}

func getContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct := &Contract{}
	err = pg.Get(ct, `
SELECT *, contracts.funds
FROM contracts
WHERE id = $1`,
		ctid)
	if err == sql.ErrNoRows {
		// couldn't find on database, maybe it's a temporary contract?
		ct, err = contractFromRedis(ctid)
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to fetch fetch contract from redis")
			jsonError(w, "", 404)
			return
		}
		err = ct.getInvoice()
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to get/make invoice")
			jsonError(w, "", 500)
			return
		}
	} else if err != nil {
		// it's a database error
		log.Warn().Err(err).Str("ctid", ctid).Msg("database error fetching contract")
		jsonError(w, "database error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: ct})
}

func getContractState(w http.ResponseWriter, r *http.Request) {
	var state types.JSONText
	err = pg.Get(&state, `SELECT state FROM contracts WHERE id = $1`, mux.Vars(r)["ctid"])
	if err != nil {
		jsonError(w, "contract not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: state})
}

func getContractFunds(w http.ResponseWriter, r *http.Request) {
	var funds int
	err = pg.Get(&funds, `SELECT contracts.funds FROM contracts WHERE id = $1`, mux.Vars(r)["ctid"])
	if err != nil {
		jsonError(w, "contract not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: funds})
}

func makeContract(w http.ResponseWriter, r *http.Request) {
	// init is a special call that enables a contract.
	// it has a fixed cost and its payload is the initial state of the contract.
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	ct, err := contractFromRedis(ctid)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch contract from redis")
		jsonError(w, "couldn't find contract "+ctid+", it may have expired", 404)
		return
	}

	_, err = checkPayment(s.ServiceId+"."+ctid, ct.getCost()+s.InitialContractFillSatoshis*1000)
	if err != nil {
		logger.Warn().Err(err).Msg("payment check failed")
		jsonError(w, "payment check failed", 402)
		return
	}

	// initiate transaction
	txn, err := pg.BeginTxx(context.TODO(),
		&sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		logger.Warn().Err(err).Msg("transaction start failed")
		jsonError(w, "", 500)
		return
	}
	defer txn.Rollback()

	// create initial contract
	_, err = txn.Exec(`
INSERT INTO contracts (id, name, readme, code, state)
VALUES ($1, $2, $3, $4, '{}')
    `, ct.Id, ct.Name, ct.Readme, ct.Code)
	if err != nil {
		log.Warn().Err(err).Str("ctid", ctid).
			Msg("failed to save contract on database")
		jsonError(w, "", 500)
		return
	}

	// instantiate call
	call := Call{
		ContractId: ct.Id,
		Id:         ct.Id, // same
		Method:     "__init__",
		Payload:    []byte{},
		Satoshis:   s.InitialContractFillSatoshis,
		Cost:       ct.getCost(),
	}

	_, err = call.runCall(txn)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to run call")
		jsonError(w, "failed to run call", 500)
		return
	}

	// saved. delete from redis.
	rds.Del("contract:" + ctid)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: nil})
}

func refillContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	sats := mux.Vars(r)["sats"]
	logger := log.With().Str("sats", sats).Str("ctid", ctid).Logger()

	label := s.ServiceId + ".refill." + ctid + "." + cuid.Slug()
	desc := s.ServiceId + " contract refill [" + ctid + "]"

	inv, err := ln.Call("invoice", sats+"000", label, desc, "36000")
	if err != nil {
		logger.Warn().Err(err).Msg("failed to generate invoice")
		jsonError(w, "failed to generate invoice", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: inv.Get("bolt11").String()})
}
