package main

import (
	"context"
	"net/http"
	"time"

	"github.com/cjoudrey/gluahttp"
	"github.com/jmoiron/sqlx"
	"github.com/tengattack/gluacrypto"
	"github.com/yuin/gopher-lua"
	gluajson "layeh.com/gopher-json"
)

func runLua(
	txn *sqlx.Tx,
	ctid string,
	method string,
	satoshis int,
	payload []byte,
) (bstate []byte, ret interface{}, err error) {
	// get contract data
	var ct Contract
	err = txn.Get(&ct, `SELECT * FROM contract WHERE id = $1`, ctid)
	if err != nil {
		return
	}

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
	gluacrypto.Preload(L)

	var currentstate map[string]interface{}
	err = ct.State.Unmarshal(&currentstate)
	if err != nil {
		return
	}

	// globals
	L.SetGlobal("state", gluajson.DecodeValue(L, currentstate))
	L.SetGlobal("payload", gluajson.DecodeValue(L, payload))
	L.SetGlobal("satoshis", lua.LNumber(satoshis))

	// run the code
	err = L.DoString(ct.Code)
	if err != nil {
		return
	}

	// call function
	err = L.CallByParam(lua.P{
		Fn:      L.GetGlobal(method),
		NRet:    1,
		Protect: true,
	})
	if err != nil {
		log.Error().Err(err).Str("method", method).Msg("error calling method")
		return
	}

	// get state after method is run
	bstate, err = gluajson.Encode(L.GetGlobal("state"))
	if err != nil {
		return
	}

	// return
	ret, err = gluajson.Encode(L.Get(-1))
	L.Pop(1)
	if err != nil {
		return
	}

	return bstate, ret, nil
}
