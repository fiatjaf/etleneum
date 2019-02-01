package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"
	gluajson "layeh.com/gopher-json"
)

func runLua(
	contract Contract,
	call Call,
) (bstate []byte, totalPaid int, paymentsPending []string, ret interface{}, err error) {
	// init lua
	L := lua.NewState()
	defer L.Close()

	// execution timeout (will cause execution to err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	L.SetContext(ctx)

	initialFunds := contract.Funds + call.Satoshis
	totalPaid = 0

	lua_ln_pay, payments_done := make_lua_ln_pay(
		L,
		func() int { return initialFunds - totalPaid },
	)

	mutex := &sync.Mutex{}
	done := make(chan bool)
	go func() {
		for payment := range payments_done {
			log.Debug().Str("ct", contract.Id).
				Float64("satoshis", payment.sats).
				Str("bolt11", payment.bolt11).
				Msg("contract trying to make payment")

			mutex.Lock()
			totalPaid += int(math.Ceil(payment.sats))
			paymentsPending = append(paymentsPending, payment.bolt11)
			mutex.Unlock()
		}
		done <- true
	}()

	lua_print, _ := make_lua_print(L)

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

	// run the code
	log.Debug().Int("satoshis", call.Satoshis).
		Interface("payload", payload).
		Interface("state", currentstate).
		Msg("running code")

	// globals
	L.SetGlobal("state", gluajson.DecodeValue(L, currentstate))
	L.SetGlobal("payload", gluajson.DecodeValue(L, payload))
	L.SetGlobal("satoshis", lua.LNumber(call.Satoshis))
	L.SetGlobal("lnpay", lua_ln_pay)
	L.SetGlobal("print", lua_print)

	bsandboxCode, _ := Asset("static/sandbox.lua")
	sandboxCode := string(bsandboxCode)
	code := fmt.Sprintf(`
%s
%s
local ln = {pay=lnpay}

local ret = sandbox.run(%s, {quota=50, env={
  print=print,
  ln=ln,
  payload=payload,
  state=state,
  satoshis=satoshis
}})
return ret
    `, sandboxCode, contract.Code, call.Method)

	// call function
	err = L.DoString(code)
	if err != nil {
		if apierr, ok := err.(*lua.ApiError); ok {
			fmt.Println(stackTraceWithCode(apierr.StackTrace, code))
			log.Error().Str("obj", apierr.Object.String()).
				Str("type", luaErrorType(apierr)).Err(apierr.Cause).
				Str("method", call.Method).Msg("")
			err = errors.New("error executing lua code")
		} else {
			log.Error().Err(err).Str("method", call.Method).Msg("error calling method")
		}
		return
	}

	// finish
	close(payments_done)
	<-done

	// get state after method is run
	bstate, err = gluajson.Encode(L.GetGlobal("state"))
	if err != nil {
		return
	}

	// returned value
	bret, err := gluajson.Encode(L.Get(-1))
	if err != nil {
		return
	}
	err = json.Unmarshal(bret, &ret)
	if err != nil {
		return
	}

	return bstate, totalPaid, paymentsPending, ret, nil
}

type payment struct {
	sats   float64
	bolt11 string
}

func make_lua_ln_pay(
	L *lua.LState,
	get_contract_funds func() int,
) (lua_ln_pay lua.LValue, payments_done chan payment) {
	payments_done = make(chan payment)

	lua_ln_pay = L.NewFunction(func(L *lua.LState) int {
		bolt11 := L.CheckString(1)
		defaults := L.NewTable()
		opts := L.OptTable(2, defaults)

		l_maxsats := opts.RawGetString("max")
		l_exact := opts.RawGetString("exact")
		l_hash := opts.RawGetString("hash")

		log.Debug().
			Interface("max", l_maxsats).
			Interface("exact", l_exact).
			Interface("hash", l_hash).
			Str("bolt11", bolt11).Msg("ln.pay called")

		var (
			invsats     float64
			invhash     string
			invexpiries time.Time
		)

		res, err := ln.Call("decodepay", bolt11)
		if err != nil {
			log.Print(err)
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// check hash filter
		invhash = res.Get("payment_hash").String()
		if l_hash != lua.LNil && lua.LVAsString(l_hash) != invhash {
			msg := "invoice hash doesn't correspond"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		invsats = res.Get("msatoshi").Float() / 1000
		// check max satoshis filter
		if l_maxsats != lua.LNil && float64(lua.LVAsNumber(l_maxsats)) < invsats {
			msg := "invoice max satoshis doesn't correspond"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check exact satoshis filter
		if l_exact != lua.LNil && float64(lua.LVAsNumber(l_exact)) != invsats {
			msg := "invoice exact satoshis doesn't correspond"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check contract funds
		if float64(get_contract_funds()) < invsats {
			msg := "contract doesn't have enough funds"
			log.Print(msg)
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check invoice expiration (should be at least 10 minutes in the future)
		invexpiries = time.Unix(res.Get("created_at").Int(), 0).Add(
			time.Second * time.Duration(res.Get("expiry").Int()),
		)
		if invexpiries.Before(time.Now().Add(time.Minute * 10)) {
			msg := "invoice is expired or about to expire"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		payments_done <- payment{sats: invsats, bolt11: bolt11}
		// actually the payments are only processed later,
		// after the contract is finished and we're able to get
		// its funds from the database.

		L.Push(lua.LNumber(invsats))
		return 1
	})

	return
}

func make_lua_print(L *lua.LState) (lua_print lua.LValue, printed []string) {
	printed = make([]string, 0)

	lua_print = L.NewFunction(func(L *lua.LState) int {
		arg := L.CheckAny(1)
		log.Debug().Str("arg", arg.String()).Msg("printing from contract")

		if v, err := gluajson.Encode(arg); err == nil {
			printed = append(printed, string(v))
		}

		return 0
	})

	return
}
