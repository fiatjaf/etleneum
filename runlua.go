package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
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

	initialFunds := contract.Funds + call.Satoshis*1000
	totalPaid = 0

	lua_ln_pay, payments_done := make_lua_ln_pay(
		L,
		func() (msats int) { return initialFunds - totalPaid },
	)

	mutex := &sync.Mutex{}
	done := make(chan bool)
	go func() {
		for payment := range payments_done {
			log.Debug().Str("ct", contract.Id).
				Int("msats", payment.msats).
				Str("bolt11", payment.bolt11).
				Msg("contract trying to make payment")

			mutex.Lock()
			totalPaid += payment.msats
			paymentsPending = append(paymentsPending, payment.bolt11)
			mutex.Unlock()
		}
		done <- true
	}()

	lua_print, _ := make_lua_print(L)
	lua_http_gettext, lua_http_getjson, _ := make_lua_http(L)

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
	L.SetGlobal("httpgettext", lua_http_gettext)
	L.SetGlobal("httpgetjson", lua_http_getjson)
	L.SetGlobal("print", lua_print)
	L.SetGlobal("sha256", L.NewFunction(lua_sha256))

	bsandboxCode, _ := Asset("static/sandbox.lua")
	sandboxCode := string(bsandboxCode)
	code := fmt.Sprintf(`
%s
%s
local ln = {pay=lnpay}
local http = {gettext=gettext, getjson=getjson}

local ret = sandbox.run(%s, {quota=50, env={
  print=print,
  sha256=sha256,
  ln=ln,
  http=http,
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

	// returned value
	bret, err := gluajson.Encode(L.Get(-1))
	if err != nil {
		return
	}
	err = json.Unmarshal(bret, &ret)
	if err != nil {
		return
	}

	// get state after method is run
	if call.Method == "__init__" {
		// on __init__ calls the returned value is the initial state
		bstate = bret
	} else {
		bstate, err = gluajson.Encode(L.GetGlobal("state"))
		if err != nil {
			return
		}
	}

	return bstate, totalPaid, paymentsPending, ret, nil
}

type payment struct {
	msats  int
	bolt11 string
}

func make_lua_ln_pay(
	L *lua.LState,
	get_contract_funds func() int, // in msats
) (lua_ln_pay lua.LValue, payments_done chan payment) {
	payments_done = make(chan payment)

	lua_ln_pay = L.NewFunction(func(L *lua.LState) int {
		bolt11 := L.CheckString(1)
		defaults := L.NewTable()
		opts := L.OptTable(2, defaults)

		l_maxsats := opts.RawGetString("max")
		l_exact := opts.RawGetString("exact")
		l_hash := opts.RawGetString("hash")
		l_payee := opts.RawGetString("payee")

		log.Debug().
			Interface("max", l_maxsats).
			Interface("exact", l_exact).
			Interface("hash", l_hash).
			Str("bolt11", bolt11).Msg("ln.pay called")

		var (
			invmsats    float64
			invhash     string
			invexpiries time.Time
			invpayee    string
		)

		res, err := ln.Call("decodepay", bolt11)
		if err != nil {
			log.Print(err)
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// check payee id filter
		invpayee = res.Get("payee").String()
		if l_payee != lua.LNil && lua.LVAsString(l_payee) != invpayee {
			msg := "invoice payee public key doesn't match"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check hash filter
		invhash = res.Get("payment_hash").String()
		if l_hash != lua.LNil && lua.LVAsString(l_hash) != invhash {
			msg := "invoice hash doesn't match"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		invmsats = res.Get("msatoshi").Float()
		invsats := invmsats / 1000
		// check max satoshis filter
		if l_maxsats != lua.LNil && float64(lua.LVAsNumber(l_maxsats)) < invsats {
			msg := "invoice max satoshis doesn't match"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check exact satoshis filter
		if l_exact != lua.LNil && float64(lua.LVAsNumber(l_exact)) != invsats {
			msg := "invoice exact satoshis doesn't match"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check contract funds
		if float64(get_contract_funds()) < invmsats {
			msg := "contract doesn't have enough funds"
			log.Print(msg)
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		// check invoice expiration (should be at least 30 minutes in the future)
		invexpiries = time.Unix(res.Get("created_at").Int(), 0).Add(
			time.Second * time.Duration(res.Get("expiry").Int()),
		)
		if invexpiries.Before(time.Now().Add(time.Minute * 30)) {
			msg := "invoice is expired or about to expire"
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(msg))
			return 2
		}

		payments_done <- payment{msats: int(invmsats), bolt11: bolt11}
		// actually the payments are only processed later,
		// after the contract is finished and we're able to get
		// its funds from the database.

		L.Push(lua.LNumber(invmsats))
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

func make_lua_http(L *lua.LState) (lua_http_gettext lua.LValue, lua_http_getjson lua.LValue, calls_p *int) {
	calls := 0
	calls_p = &calls

	http_get := func(url string, lheaders lua.LValue) (b []byte, err error) {
		log.Debug().Str("url", url).Msg("http call from contract")

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return
		}

		if lheaders != nil {
			lheaders.(*lua.LTable).ForEach(func(k lua.LValue, v lua.LValue) {
				req.Header.Set(lua.LVAsString(k), lua.LVAsString(v))
			})
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}

		if resp.StatusCode >= 400 {
			err = errors.New("response status code: " + strconv.Itoa(resp.StatusCode))
			return
		}

		b, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}

		return b, nil
	}

	lua_http_gettext = L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		lheaders := L.CheckTable(2)
		respbytes, err := http_get(url, lheaders)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LString(string(respbytes)))
		return 1
	})

	lua_http_getjson = L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		lheaders := L.CheckTable(2)
		respbytes, err := http_get(url, lheaders)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		lv, err := gluajson.Decode(L, respbytes)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lv)
		return 1
	})

	return
}

func lua_sha256(L *lua.LState) int {
	h := sha256.New()
	s := lua.LVAsString(L.Get(1))
	raw := lua.LVAsBool(L.Get(2))
	_, err := h.Write([]byte(s))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	var result string
	if !raw {
		result = hex.EncodeToString(h.Sum(nil))
	} else {
		result = string(h.Sum(nil))
	}
	L.Push(lua.LString(result))
	return 1
}
