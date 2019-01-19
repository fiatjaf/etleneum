package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/cjoudrey/gluahttp"
	"github.com/yuin/gopher-lua"
	gluajson "layeh.com/gopher-json"
)

func runLua(
	contract int,
	method string,
	invoicelabel string,
	payload map[string]interface{},
) (interface{}, error) {
	// get contract data
	var ct Contract
	err := pg.Get(&ct, `
SELECT id, code, state
FROM contract
WHERE id = $1
    `, contract)
	if err != nil {
		return nil, err
	}

	// encode payload for later
	bpayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// get invoice data
	inv, err := ln.Call("waitinvoice", invoicelabel)
	if err != nil {
		return nil, err
	}
	hash := inv.Get("payment_hash").String()
	satoshis := int(inv.Get("msatoshi_received").Float() / 1000)

	// init lua
	L := lua.NewState()
	defer L.Close()

	// execution timeout (will cause execution to err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	L.SetContext(ctx)

	// modules
	L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
	gluajson.Preload(L)

	var state map[string]interface{}
	err = ct.State.Unmarshal(&state)
	if err != nil {
		return nil, err
	}

	// globals
	L.SetGlobal("state", gluajson.DecodeValue(L, state))
	L.SetGlobal("payload", gluajson.DecodeValue(L, payload))
	L.SetGlobal("satoshis", lua.LNumber(satoshis))

	// run the code
	err = L.DoString(ct.Code)
	if err != nil {
		return nil, err
	}

	// call function
	err = L.CallByParam(lua.P{
		Fn:      L.GetGlobal(method),
		NRet:    1,
		Protect: true,
	})
	if err != nil {
		log.Error().Err(err).Str("method", method).Msg("error calling method")
		return nil, err
	}

	// get state after method is run
	bstate, err := gluajson.Encode(L.GetGlobal("state"))
	if err != nil {
		return nil, err
	}

	// save everything in the database
	_, err = pg.Exec(`
WITH contract AS (
  UPDATE contract
  SET state = $2
  WHERE id = $1
  RETURNING id
)
INSERT INTO call (label, hash, contract_id, method, payload, satoshis)
VALUES ($3, $4, (SELECT id FROM contract), $5, $6, $7)
    `, ct.Id, bstate,
		invoicelabel, hash, method, bpayload, satoshis)
	if err != nil {
		return nil, err
	}

	// returned value (return this to the caller)
	ret, err := gluajson.Encode(L.Get(-1))
	L.Pop(1)
	if err != nil {
		return nil, err
	}

	return ret, nil
}
