package runlua

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aarzilli/golua/lua"
	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/lunatico"
	"github.com/lucsky/cuid"
	"github.com/rs/zerolog"
)

var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})

func RunCall(
	sandboxCode string,
	printToDestination io.Writer,
	makeRequest func(*http.Request) (*http.Response, error),
	getExternalContractData func(string) (interface{}, int, error),
	getContractFunds func() (int, error),
	sendFromContract func(target string, sats int) (int, error),
	getCurrentAccountBalance func() (int, error),
	sendFromCurrentAccount func(target string, sats int) (int, error),
	contract types.Contract,
	call types.Call,
) (stateAfter interface{}, err error) {
	completedOk := make(chan bool, 1)
	failed := make(chan error, 1)

	go func() {
		stateAfter, err = runCall(
			sandboxCode,
			printToDestination,
			makeRequest,
			getExternalContractData,
			getContractFunds,
			sendFromContract,
			getCurrentAccountBalance,
			sendFromCurrentAccount,
			contract,
			call,
		)
		if err != nil {
			failed <- err
			return
		}

		completedOk <- true
	}()

	select {
	case <-completedOk:
		return
	case failure := <-failed:
		err = failure
		return
	case <-time.After(time.Second * 3):
		err = errors.New("timeout!")
		return
	}
}

func runCall(
	sandboxCode string,
	printToDestination io.Writer,
	makeRequest func(*http.Request) (*http.Response, error),
	getExternalContractData func(string) (interface{}, int, error),
	getContractFunds func() (int, error),
	sendFromContract func(target string, sats int) (int, error),
	getCurrentAccountBalance func() (int, error),
	sendFromCurrentAccount func(target string, sats int) (int, error),
	contract types.Contract,
	call types.Call,
) (stateAfter interface{}, err error) {
	// init lua
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs()

	initialFunds := contract.Funds + call.Msatoshi

	lua_http_gettext, lua_http_getjson, lua_http_postjson, _ := make_lua_http(makeRequest)
	var lua_current_account interface{}
	if call.Caller != "" {
		lua_current_account = call.Caller
	}

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
	log.Debug().Str("method", call.Method).
		Str("caller", call.Caller).
		Int("msatoshi", call.Msatoshi).
		Interface("payload", payload).
		Interface("state", currentstate).
		Int("funds", initialFunds).
		Msg("running code")

	// globals
	lunatico.SetGlobals(L, map[string]interface{}{
		"state":                       currentstate,
		"payload":                     payload,
		"msatoshi":                    call.Msatoshi,
		"call":                        call.Id,
		"current_account":             lua_current_account,
		"send_from_current_account":   sendFromCurrentAccount,
		"get_current_account_balance": getCurrentAccountBalance,
		"get_external_contract_data":  getExternalContractData,
		"contract":                    contract.Id,
		"get_contract_funds":          getContractFunds,
		"send_from_contract":          sendFromContract,
		"httpgettext":                 lua_http_gettext,
		"httpgetjson":                 lua_http_getjson,
		"httppostjson":                lua_http_postjson,
		"keybase_verify":              lua_keybase_verify_signature,
		"keybase_verify_bundle":       lua_keybase_verify_bundle,
		"keybase_extract_message":     lua_keybase_extract_message,
		"keybase_lookup":              lua_keybase_lookup,
		"print": func(args ...interface{}) {
			actualArgs := make([]interface{}, len(args)*2+1)
			i := 0
			for _, arg := range args {
				actualArgs[i] = arg
				actualArgs[i+1] = "\t"
				i += 2
			}
			actualArgs[i] = "\n"
			fmt.Fprint(printToDestination, actualArgs...)
		},
		"sha256": lua_sha256,
		"cuid":   cuid.New,
	})

	code := fmt.Sprintf(`
%s

-- account.id will be nil if there's not a logged user
local account_id = nil
if current_account ~= "" then
  account_id = current_account
end

custom_env = {
  http={
    gettext=httpgettext,
    getjson=httpgetjson,
    postjson=httppostjson
  },
  util={
    sha256=sha256,
    cuid=cuid,
    print=print,
  },
  contract={
    id=current_contract,
    get_funds=function ()
      funds, err = internal.get_contract_funds()
      if err ~= nil then
        error(err)
      end
      return funds
    end,
    send=function (target, amount)
      amt, err = internal.send_from_contract(target, amount)
      if err ~= nil then
        error(err)
      end
      return amt
    end,
    state=state
  },
  etleneum={
    get_contract=function (id)
      state, funds, err = internal.get_external_contract_data(id)
      if err ~= nil then
        error(err)
      end
      return state, funds
    end
  },
  account={
    id=account_id,
    send=function (target, amount)
      amt, err = internal.send_from_current_account(target, amount)
      if err ~= nil then
        error(err)
      end
      return amt
    end,
    get_balance=function ()
      balance, err = internal.get_current_account_balance()
      if err ~= nil then
        error(err)
      end
      return balance
    end,
  },
  call={
    id=call,
    payload=payload,
    msatoshi=msatoshi
  },
  keybase={
    verify=function (username, text_or_bundle, signature_block)
      if not signature_block then
        return keybase._verify_bundle(username, text_or_bundle)
      end
      return keybase._verify(username, text_or_bundle, signature_block)
    end,
    extract_message=keybase_extract_message,
    lookup=keybase_lookup,
    exists=function (n) return keybase.username(n) ~= "" end,
    github=function (n) return keybase.lookup("github", n) end,
    twitter=function (n) return keybase.lookup("twitter", n) end,
    reddit=function (n) return keybase.lookup("reddit", n) end,
    hackernews=function (n) return keybase.lookup("hackernews", n) end,
    key_fingerprint=function (n) return keybase.lookup("key_fingerprint", n) end,
    domain=function (n) return keybase.lookup("domain", n) end,
    username=function (n) return keybase.lookup("usernames", n) end,
    _verify=keybase_verify,
    _verify_bundle=keybase_verify_bundle,
  },
  internal={
    get_external_contract_data=get_external_contract_data,
    send_from_current_account=send_from_current_account,
    send_from_contract=send_from_contract,
    get_current_account_balance=get_current_account_balance,
    get_contract_funds=get_contract_funds,
  }
}

for k, v in pairs(custom_env) do
  sandbox_env[k] = v
end

function call ()
%s

  return %s()
end

ret = run(sandbox_env, call)
    `, sandboxCode, contract.Code, call.Method)

	err = L.DoString(code)
	if err != nil {
		st := stackTraceWithCode(err.Error(), code)
		log.Print(st)
		err = errors.New(st)
		return
	}

	globalsAfter := lunatico.GetGlobals(L, "ret", "state")
	stateAfter = globalsAfter["state"]

	// get state after method is run
	if call.Method == "__init__" {
		// on __init__ calls the returned value is the initial state
		stateAfter = globalsAfter["ret"]
	}

	return stateAfter, nil
}
