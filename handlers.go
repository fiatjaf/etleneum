package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func makeContract(w http.ResponseWriter, r *http.Request) {
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

	var id int
	err = pg.Get(&id, `SELECT nextval(pg_get_serial_sequence('contract', 'id'))`)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get id for temporary contract.")
		http.Error(w, "", 500)
		return
	}
	ct.Id = id
	err = ct.makeHashid()
	if err != nil {
		log.Warn().Err(err).Msg("failed to make hashid.")
		http.Error(w, "", 500)
		return
	}

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

	err = rds.Set("contract:"+ct.Hashid, jct, time.Hour*30).Err()
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		http.Error(w, "", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jct)
}

func getContract(w http.ResponseWriter, r *http.Request) {
	hashid := mux.Vars(r)["hashid"]

	ct := &Contract{Hashid: hashid}
	err := ct.makeId()
	if err != nil {
		log.Warn().Err(err).Str("hashid", hashid).Msg("failed to get id from hashid.")
		http.Error(w, "", 400)
		return
	}

	err = pg.Get(ct, `SELECT * FROM contract WHERE id = $1`, ct.Id)
	if err != nil {
		if err == sql.ErrNoRows {
			// couldn't find on database, maybe it's a temporary contract?
			jct, err := rds.Get("contract:" + hashid).Bytes()
			if err != nil {
				log.Warn().Err(err).Str("hashid", hashid).Int("id", ct.Id).
					Msg("failed to fetch contract")
				http.Error(w, "", 404)
				return
			}
			err = json.Unmarshal(jct, ct)
			if err != nil {
				log.Warn().Err(err).Str("hashid", hashid).Str("b", string(jct)).
					Msg("failed to fetch unmarshal contract from redis")
				http.Error(w, "", 500)
				return
			}
			err = ct.getInvoice()
			if err != nil {
				log.Warn().Err(err).Str("hashid", hashid).
					Msg("failed to get/make invoice")
				http.Error(w, "", 500)
				return
			}
		} else {
			// it's a database error
			log.Warn().Err(err).Str("hashid", hashid).Int("id", ct.Id).
				Msg("database error fetching contract")
			http.Error(w, "", 500)
			return
		}
	} else {
		// found on database
		ct.Hashid = hashid
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ct)
}

func initContract(w http.ResponseWriter, r *http.Request) {
	// init is a special call that enables a contract.
	// it has a fixed cost and its payload is the initial state of the contract.
	hashid := mux.Vars(r)["hashid"]
	costsatoshis := s.InitCostSatoshis

	jct, err := rds.Get("contract:" + hashid).Bytes()
	if err != nil {
		log.Warn().Err(err).Str("hashid", hashid).
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

	err = ct.makeId()
	if err != nil {
		log.Warn().Err(err).Str("hashid", hashid).
			Msg("failed to make id from hashid on init")
		http.Error(w, "", 500)
		return
	}

	// check payment
	var (
		invlabel string
		invpaid  int
		invhash  string
	)

	invlabel = s.ServiceId + "." + hashid
	res, err := ln.Call("listinvoices", invlabel)
	if err != nil {
		log.Warn().Err(err).Str("hashid", hashid).
			Msg("failed to get invoice from lightningd")
		http.Error(w, "", 500)
		return
	}

	invpaid = int(res.Get("invoices.0.msatoshi_received").Int() / 1000)
	invhash = res.Get("invoices.0.payment_hash").String()
	if res.Get("invoices.0.status").String() != "paid" || invpaid < costsatoshis {
		log.Warn().Err(err).Str("hashid", hashid).Str("invoice", res.String()).
			Msg("invoice not paid")
		http.Error(w, "", 402)
		return
	}

	// request payload should be the initial state
	bpayload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Warn().Err(err).Str("hashid", hashid).
			Msg("failed to read init payload")
		http.Error(w, "", 400)
		return
	}
	payload := string(bpayload)

	_, err = pg.Exec(`
WITH contract AS (
  INSERT INTO contract (id, name, readme, code, state)
  VALUES ($1, $2, $3, $4, $5)
  RETURNING id
)
INSERT INTO call (hash, label, contract_id, method, payload, satoshis)
VALUES ($6, $7, (SELECT id FROM contract), '__init__', $8, $9)
    `, ct.Id, ct.Name, ct.Readme, ct.Code, payload,
		invhash, invlabel, payload, invpaid)
	if err != nil {
		log.Warn().Err(err).Str("hashid", hashid).
			Msg("failed to save contract on database")
		http.Error(w, "", 500)
		return
	}

	// saved. delete from redis.
	rds.Del("contract:" + hashid)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok": true}`))
}

func getCall(w http.ResponseWriter, r *http.Request) {

}

func makeCall(w http.ResponseWriter, r *http.Request) {

}
