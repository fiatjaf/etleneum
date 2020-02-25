package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/fiatjaf/etleneum/types"
	"github.com/gorilla/mux"
	sqlxtypes "github.com/jmoiron/sqlx/types"
	"github.com/lucsky/cuid"
)

func listContracts(w http.ResponseWriter, r *http.Request) {
	contracts := make([]types.Contract, 0)
	err = pg.Select(&contracts, `
SELECT id, name, readme, funds, ncalls FROM (
  SELECT `+types.CONTRACTFIELDS+`, c.funds,
    (SELECT max(time) FROM calls WHERE contract_id = c.id) AS lastcalltime,
    (SELECT count(*) FROM calls WHERE contract_id = c.id) AS ncalls
  FROM contracts AS c
) AS x
ORDER BY lastcalltime DESC, created_at DESC
    `)
	if err == sql.ErrNoRows {
		contracts = make([]types.Contract, 0)
	} else if err != nil {
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
	ct := &types.Contract{}
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

	var invoice string
	if s.FreeMode {
		invoice = BOGUS_INVOICE

		// wait 10 seconds and notify this payment was received
		go func() {
			time.Sleep(5 * time.Second)
			contractPaymentReceived(ct.Id, getContractCost(*ct))
		}()
	} else {
		invoice, err = makeInvoice(
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
	}

	_, err = saveContractOnRedis(*ct)
	if err != nil {
		log.Warn().Err(err).Interface("ct", ct).Msg("failed to save to redis")
		jsonError(w, "failed to save prepared contract", 500)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: types.StuffBeingCreated{
		Id:      ct.Id,
		Invoice: invoice,
	}})
}

func getContract(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]

	ct := &types.Contract{}
	err = pg.Get(ct, `
SELECT `+types.CONTRACTFIELDS+`, contracts.funds
FROM contracts
WHERE id = $1`,
		ctid)
	if err == sql.ErrNoRows {
		// couldn't find on database, maybe it's a temporary contract?
		ct, err = contractFromRedis(ctid)
		if err != nil {
			log.Warn().Err(err).Str("ctid", ctid).
				Msg("failed to fetch fetch prepared contract from redis")
			jsonError(w, "failed to fetch prepared contract", 404)
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
	var state sqlxtypes.JSONText
	err = pg.Get(&state, `SELECT state FROM contracts WHERE id = $1`, mux.Vars(r)["ctid"])
	if err != nil {
		jsonError(w, "contract not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if jqfilter, ok := mux.Vars(r)["jq"]; ok && strings.TrimSpace(jqfilter) != "" {
		if result, err := runJQ(r.Context(), []byte(state), jqfilter); err != nil {
			log.Warn().Err(err).Str("f", jqfilter).Str("state", string(state)).
				Msg("error applying jq filter")
			jsonError(w, "error applying jq filter", 400)
			return
		} else {
			jresult, _ := json.Marshal(result)
			state = sqlxtypes.JSONText(jresult)
		}
	}

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

func deleteContract(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["ctid"]

	var err error

	// can only delete on free mode
	if s.FreeMode {
		_, err = pg.Exec(`
WITH del_t AS (
  DELETE FROM internal_transfers
  WHERE call_id IN (SELECT id FROM calls WHERE contract_id = $1)
), del_c AS (
  DELETE FROM calls WHERE contract_id = $1
)
DELETE FROM contracts WHERE id = $1
    `, id)
	}
	if err != nil {
		log.Info().Err(err).Str("id", id).Msg("can't delete contract")
		jsonError(w, "can't delete contract", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true})
}

func listEvents(w http.ResponseWriter, r *http.Request) {
	ctid := mux.Vars(r)["ctid"]
	logger := log.With().Str("ctid", ctid).Logger()

	events := make([]types.ContractEvent, 0)
	err = pg.Select(&events, `
SELECT `+types.CONTRACTEVENTFIELDS+`
FROM contract_events
WHERE contract = $1
ORDER BY time DESC
LIMIT 50
    `, ctid)

	if err == sql.ErrNoRows {
		events = make([]types.ContractEvent, 0)
	} else if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch events")
		jsonError(w, "failed to fetch events", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Result{Ok: true, Value: events})
}
