package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/fiatjaf/etleneum/data"
	"github.com/gorilla/mux"
	"github.com/lucsky/cuid"
)

func listContracts(w http.ResponseWriter, r *http.Request) {
	contracts, err := data.ListContracts()
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch contracts")
		jsonError(w, "failed to fetch contracts", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: contracts})
}

func prepareContract(w http.ResponseWriter, r *http.Request) {
	// making a contract only saves it temporarily.
	// the contract can be inspected only by its creator.
	// once the creator knows everything is right, he can call init.
	ct := &data.Contract{}
	err := json.NewDecoder(r.Body).Decode(ct)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse contract json")
		jsonError(w, "failed to parse json", 400)
		return
	}
	if strings.TrimSpace(ct.Name) == "" {
		jsonError(w, "contract must have a name", 400)
		return
	}

	ct.Id = "c" + cuid.Slug()

	if ok := checkContractCode(ct.Code); !ok {
		log.Warn().Err(err).Msg("invalid contract code")
		jsonError(w, "invalid contract code", 400)
		return
	}

	invoice, err := makeInvoice(
		s.FreeMode,
		ct.Id,
		ct.Id,
		s.ServiceId+" __init__ ["+ct.Id+"]",
		nil,
		getContractCost(*ct),
		0,
	)
	if err != nil {
		log.Warn().Err(err).Msg("failed to make invoice.")
		jsonError(w, "failed to make invoice", 500)
		return
	}
	if s.FreeMode {
		// wait 10 seconds and notify this payment was received
		go func() {
			time.Sleep(5 * time.Second)
			contractPaymentReceived(ct.Id, getContractCost(*ct))
		}()
	}

	_, err = saveContractOnRedis(*ct)
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		jsonError(w, "failed to save prepared contract", 500)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: map[string]interface{}{
		"id":      ct.Id,
		"invoice": invoice,
	}})
}

func getContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct, err := data.GetContract(ctid)
	if err != nil {
		// it's a database error
		log.Warn().Err(err).Str("ctid", ctid).Msg("database error fetching contract")
		jsonError(w, "database error", 500)
		return
	}

	if ct == nil {
		// couldn't find on database, maybe it's a temporary contract?
		ct, err = contractFromRedis(ctid)
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to fetch fetch prepared contract from redis")
			jsonError(w, "failed to fetch prepared contract", 404)
			return
		}
	}

	if ct.Methods == nil {
		ct.Methods = make([]data.Method, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: ct})
}

func getContractState(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct, _ := data.GetContract(ctid)
	if ct == nil {
		jsonError(w, "contract not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var jqfilter string
	if r.Method == "GET" {
		jqfilter, _ = mux.Vars(r)["jq"]
	} else if r.Method == "POST" {
		defer r.Body.Close()
		b, _ := ioutil.ReadAll(r.Body)
		jqfilter = string(b)
	}

	state := ct.State
	if strings.TrimSpace(jqfilter) != "" {
		if result, err := runJQ(r.Context(), state, jqfilter); err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Str("f", jqfilter).Str("state", string(state)).
				Msg("error applying jq filter")
			jsonError(w, "error applying jq filter", 400)
			return
		} else {
			jresult, _ := json.Marshal(result)
			state = json.RawMessage(jresult)
		}
	}

	json.NewEncoder(w).Encode(Result{Ok: true, Value: state})
}

func getContractFunds(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct, _ := data.GetContract(ctid)
	if ct == nil {
		jsonError(w, "contract not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: ct.Funds})
}

func deleteContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	var err error

	// can only delete on free mode
	if s.FreeMode {
		err = data.DeleteContract(ctid)
	} else {
		err = errors.New("only works on free mode")
	}

	if err != nil {
		log.Info().Err(err).Str("id", ctid).Msg("can't delete contract")
		jsonError(w, "can't delete contract", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true})
}
