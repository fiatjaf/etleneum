package main

import (
	"context"
	"time"

	"github.com/tengattack/gluacrypto"
	"github.com/yuin/gopher-lua"
	gluajson "layeh.com/gopher-json"
)

func runLua(
	contract Contract,
	call Call,
) (bstate []byte, ret interface{}, err error) {
	// init lua
	L := lua.NewState()
	defer L.Close()

	// execution timeout (will cause execution to err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	L.SetContext(ctx)

	// modules
	gluajson.Preload(L)
	gluacrypto.Preload(L)

	var currentstate map[string]interface{}
	err = contract.State.Unmarshal(&currentstate)
	if err != nil {
		return
	}

	var payload map[string]interface{}
	err = call.Payload.Unmarshal(&payload)
	if err != nil {
		return
	}

	// globals
	L.SetGlobal("state", gluajson.DecodeValue(L, currentstate))
	L.SetGlobal("payload", gluajson.DecodeValue(L, payload))
	L.SetGlobal("satoshis", lua.LNumber(call.Satoshis))

	// run the code
	log.Debug().Int("satoshis", call.Satoshis).
		Interface("payload", payload).
		Interface("state", currentstate).
		Msg("running code")
	err = L.DoString(contract.Code)
	if err != nil {
		return
	}

	// call function
	log.Print("calling function ", call.Method)
	err = L.CallByParam(lua.P{
		Fn:      L.GetGlobal(call.Method),
		NRet:    1,
		Protect: true,
	})
	if err != nil {
		log.Error().Err(err).Str("method", call.Method).
			Msg("error calling method")
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
