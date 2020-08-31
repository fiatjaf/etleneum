package runlua

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aarzilli/golua/lua"
	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/lunatico"
	"github.com/lucsky/cuid"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func RunCall(
	logger zerolog.Logger,
	printToDestination io.Writer,
	makeRequest func(*http.Request) (*http.Response, error),
	getExternalContractData func(string) (interface{}, int64, error),
	callExternalMethod func(string, string, interface{}, int64, string) error,
	getContractFunds func() (int, error),
	sendFromContract func(target string, sats int) (int, error),
	getCurrentAccountBalance func() (int, error),
	sendFromCurrentAccount func(target string, sats int) (int, error),
	contract types.Contract,
	call types.Call,
) (stateAfter interface{}, err error) {
	log = logger
	completedOk := make(chan bool, 1)
	failed := make(chan error, 1)

	go func() {
		stateAfter, err = runCall(
			printToDestination,
			makeRequest,
			getExternalContractData,
			callExternalMethod,
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
	printToDestination io.Writer,
	makeRequest func(*http.Request) (*http.Response, error),
	getExternalContractData func(string) (interface{}, int64, error),
	callExternalMethod func(string, string, interface{}, int64, string) error,
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
		Int64("msatoshi", call.Msatoshi).
		Interface("payload", payload).
		Interface("state", currentstate).
		Int64("funds", initialFunds).
		Msg("running code")

	actualCode := contract.Code + "\nreturn " + call.Method + "()"

	// globals
	lunatico.SetGlobals(L, map[string]interface{}{
		"code":                        actualCode,
		"state":                       currentstate,
		"payload":                     payload,
		"msatoshi":                    call.Msatoshi,
		"call":                        call.Id,
		"current_contract":            call.ContractId,
		"current_account":             lua_current_account,
		"send_from_current_account":   sendFromCurrentAccount,
		"get_current_account_balance": getCurrentAccountBalance,
		"get_external_contract_data":  getExternalContractData,
		"call_external_method":        callExternalMethod,
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
				var v interface{}
				switch arg.(type) {
				case string, int, int64, float64, bool:
					v = arg
				default:
					j, _ := json.Marshal(arg)
					v = string(j)
				}

				actualArgs[i] = v
				actualArgs[i+1] = "\t"
				i += 2
			}
			actualArgs[i] = "\n"
			fmt.Fprint(printToDestination, actualArgs...)
		},
		"sha256":       lua_sha256,
		"cuid":         cuid.Slug,
		"parse_bolt11": lua_parse_bolt11,
	})

	code := `
-- account.id will be nil if there's not a logged user
local account_id = nil
if current_account ~= "" then
  account_id = current_account
end

sandbox_env = {
  ipairs = ipairs,
  next = next,
  pairs = pairs,
  error = error,
  tonumber = tonumber,
  tostring = tostring,
  type = type,
  unpack = unpack,
  utf8 = utf8,
  string = { byte = string.byte, char = string.char, find = string.find,
      format = string.format, gmatch = string.gmatch, gsub = string.gsub,
      len = string.len, lower = string.lower, match = string.match,
      rep = string.rep, reverse = string.reverse, sub = string.sub,
      upper = string.upper },
  table = { insert = table.insert, maxn = table.maxn, remove = table.remove,
      sort = table.sort, pack = table.pack },
  math = { abs = math.abs, acos = math.acos, asin = math.asin,
      atan = math.atan, atan2 = math.atan2, ceil = math.ceil, cos = math.cos,
      cosh = math.cosh, deg = math.deg, exp = math.exp, floor = math.floor,
      fmod = math.fmod, frexp = math.frexp, huge = math.huge,
      ldexp = math.ldexp, log = math.log, log10 = math.log10, max = math.max,
      min = math.min, modf = math.modf, pi = math.pi, pow = math.pow,
      rad = math.rad, random = math.random, randomseed = math.randomseed,
      sin = math.sin, sinh = math.sinh, sqrt = math.sqrt, tan = math.tan, tanh = math.tanh },
  os = { clock = os.clock, difftime = os.difftime, time = os.time, date = os.date },
  http = {
    gettext = httpgettext,
    getjson = httpgetjson,
    postjson = httppostjson
  },
  util = {
    sha256 = sha256,
    cuid = cuid,
    print = print,
    parse_bolt11 = parse_bolt11,
  },
  contract = {
    id = current_contract,
    get_funds = function ()
      funds, err = get_contract_funds()
      if err ~= nil then
        error(err)
      end
      return funds
    end,
    send = function (target, amount)
      amt, err = send_from_contract(target, amount)
      if err ~= nil then
        error(err)
      end
      return amt
    end,
    state = state
  },
  etleneum = {
    get_contract = function (id)
      state, funds, err = get_external_contract_data(id)
      if err ~= nil then
        error(err)
      end
      return state, funds
    end,
    call_external = function (contract, method, payload, msatoshi, params)
      local as = nil
      if account_id and params.as == 'caller' then
        as = account_id
      elseif params.as == 'contract' then
        as = current_contract
      end
      local err = call_external_method(contract, method, payload, msatoshi, as)
      if err ~= nil then
        error(err)
      end
    end
  },
  account = {
    id = account_id,
    send = function (target, amount)
      amt, err = send_from_current_account(target, amount)
      if err ~= nil then
        error(err)
      end
      return amt
    end,
    get_balance = function ()
      balance, err = get_current_account_balance()
      if err ~= nil then
        error(err)
      end
      return balance
    end,
  },
  call = {
    id = call,
    payload = payload,
    msatoshi = msatoshi
  },
  keybase = {
    verify = function (username, text_or_bundle, signature_block)
      if not signature_block then
        return keybase_verify_bundle(username, text_or_bundle)
      end
      return keybase_verify(username, text_or_bundle, signature_block)
    end,
    extract_message = keybase_extract_message,
    lookup = keybase_lookup,
    exists = function (n) return keybase.username(n) ~= "" end,
    github = function (n) return keybase.lookup("github", n) end,
    twitter = function (n) return keybase.lookup("twitter", n) end,
    reddit = function (n) return keybase.lookup("reddit", n) end,
    hackernews = function (n) return keybase.lookup("hackernews", n) end,
    key_fingerprint = function (n) return keybase.lookup("key_fingerprint", n) end,
    domain = function (n) return keybase.lookup("domain", n) end,
    username = function (n) return keybase.lookup("usernames", n) end,
    _verify = keybase_verify,
    _verify_bundle = keybase_verify_bundle,
  }
}

_calls = 0
function count ()
  _calls = _calls + 1
  if _calls > 1000 then
    error('too many operations!')
  end
end
debug.sethook(count, 'c')

ret = load(code, 'call', 't', sandbox_env)()
state = sandbox_env.contract.state
    `

	err = L.DoString(code)
	if err != nil {
		st := stackTraceWithCode(err.Error(), actualCode)
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
