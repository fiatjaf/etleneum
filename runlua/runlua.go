package runlua

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aarzilli/golua/lua"
	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/lunatico"
	"github.com/rs/zerolog"
	"github.com/tidwall/gjson"
)

var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})

func RunCall(
	sandboxCode string,
	parseInvoice func(string) (gjson.Result, error),
	contract types.Contract,
	call types.Call,
) (stateAfter interface{}, totalPaid int, paymentsPending []string, returnedValue interface{}, err error) {
	// init lua
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs()

	initialFunds := contract.Funds + call.Satoshis*1000
	totalPaid = 0

	lua_ln_pay, payments_done := make_lua_ln_pay(
		func() (msats int) { return initialFunds - totalPaid },
		parseInvoice,
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

	lua_http_gettext, lua_http_getjson, _ := make_lua_http()

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
		Int("satoshis", call.Satoshis).
		Interface("payload", payload).
		Interface("state", currentstate).
		Msg("running code")

	// globals
	lunatico.SetGlobals(L, map[string]interface{}{
		"state":       currentstate,
		"payload":     payload,
		"satoshis":    call.Satoshis,
		"lnpay":       lua_ln_pay,
		"httpgettext": lua_http_gettext,
		"httpgetjson": lua_http_getjson,
		"print":       func(args ...interface{}) { fmt.Println(args...) },
		"sha256":      lua_sha256,
	})

	code := fmt.Sprintf(`
%s

custom_env = {
  print=print,
  http={
    gettext=httpgettext,
    getjson=httpgetjson
  },
  util={
    sha256=sha256
  },
  ln={pay=lnpay},
  payload=payload,
  state=state,
  satoshis=satoshis
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
		log.Print(stackTraceWithCode(err.Error(), code))
		return
	}

	globalsAfter := lunatico.GetGlobals(L, "ret", "state")
	stateAfter = globalsAfter["state"]
	returnedValue = globalsAfter["ret"]

	// finish
	close(payments_done)
	<-done

	// get state after method is run
	if call.Method == "__init__" {
		// on __init__ calls the returned value is the initial state
		stateAfter = returnedValue
	}

	return stateAfter, totalPaid, paymentsPending, returnedValue, nil
}

type payment struct {
	msats  int
	bolt11 string
}

func make_lua_ln_pay(
	get_contract_funds func() int, // in msats
	parse_invoice func(string) (gjson.Result, error),
) (
	lua_ln_pay func(string, ...map[string]interface{}) (int, error),
	payments_done chan payment,
) {
	payments_done = make(chan payment)

	lua_ln_pay = func(bolt11 string, filters ...map[string]interface{}) (int, error) {
		filter := make(map[string]interface{})
		for _, f := range filters {
			for attr, value := range f {
				filter[attr] = value
			}
		}

		log.Debug().Interface("filter", filter).Str("bolt11", bolt11).Msg("ln.pay called")

		var (
			invmsats    float64
			invhash     string
			invexpiries time.Time
			invpayee    string
		)

		res, err := parse_invoice(bolt11)
		if err != nil {
			log.Debug().Err(err).Str("bolt11", bolt11).Msg("failed to parse invoice")
			err = errors.New("failed to parse invoice: " + err.Error())
			return 0, err
		}

		// check payee id filter
		invpayee = res.Get("payee").String()
		if f_payee, set := filter["payee"]; set {
			if v_payee, ok := f_payee.(string); ok && v_payee != invpayee {
				err := fmt.Errorf("invoice payee public key doesn't match: %s != %s",
					v_payee, invpayee)
				log.Print(err)
				return 0, err
			}
		}

		// check hash filter
		invhash = res.Get("payment_hash").String()
		if f_hash, set := filter["hash"]; set {
			if v_hash, ok := f_hash.(string); ok && v_hash != invhash {
				err := fmt.Errorf("invoice hash doesn't match: %s != %s", v_hash, invhash)
				log.Print(err)
				return 0, err
			}
		}

		invmsats = res.Get("msatoshi").Float()
		invsats := invmsats / 1000

		// check max satoshis filter
		if f_max, set := filter["max"]; set {
			if v_max, ok := f_max.(float64); ok && v_max != invsats {
				err := fmt.Errorf("invoice max satoshis doesn't match: %f < %f", v_max, invsats)
				log.Print(err)
				return 0, err
			}
		}
		// check exact satoshis filter
		if f_exact, set := filter["exact"]; set {
			if v_exact, ok := f_exact.(float64); ok && v_exact != invsats {
				err := fmt.Errorf("invoice exact satoshis doesn't match: %f != %f", v_exact, invsats)
				log.Print(err)
				return 0, err
			}
		}

		// check contract funds
		if float64(get_contract_funds()) < invmsats {
			err := fmt.Errorf("contract doesn't have enough funds.")
			log.Print(err)
			return 0, err
		}

		// check invoice expiration (should be at least 30 minutes in the future)
		invexpiries = time.Unix(res.Get("created_at").Int(), 0).Add(
			time.Second * time.Duration(res.Get("expiry").Int()),
		)
		if invexpiries.Before(time.Now().Add(time.Minute * 30)) {
			err := fmt.Errorf("invoice is expired or about to expire")
			return 0, err
		}

		payments_done <- payment{msats: int(invmsats), bolt11: bolt11}
		// actually the payments are only processed later,
		// after the contract is finished and we're able to get
		// its funds from the database.

		return int(invmsats), nil
	}

	return
}
func make_lua_http() (
	lua_http_gettext func(string, ...map[string]string) (string, error),
	lua_http_getjson func(string, ...map[string]string) (interface{}, error),
	calls_p *int,
) {
	calls := 0
	calls_p = &calls

	http_get := func(url string, headers ...map[string]string) (b []byte, err error) {
		log.Debug().Str("url", url).Msg("http call from contract")

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return
		}

		for _, headermap := range headers {
			for k, v := range headermap {
				req.Header.Set(k, v)
			}
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

	lua_http_gettext = func(url string, headers ...map[string]string) (t string, err error) {
		respbytes, err := http_get(url, headers...)
		if err != nil {
			return "", err
		}
		return string(respbytes), nil
	}

	lua_http_getjson = func(url string, headers ...map[string]string) (j interface{}, err error) {
		respbytes, err := http_get(url, headers...)
		if err != nil {
			return nil, err
		}

		var value interface{}
		err = json.Unmarshal(respbytes, &value)
		if err != nil {
			return nil, err
		}

		return value, nil
	}

	return
}

func lua_sha256(preimage string) (hash string, err error) {
	h := sha256.New()
	_, err = h.Write([]byte(preimage))
	if err != nil {
		return "", err
	}
	hash = hex.EncodeToString(h.Sum(nil))
	return hash, nil
}

var reNumber = regexp.MustCompile("\\d+")

func stackTraceWithCode(stacktrace string, code string) string {
	var result []string

	stlines := strings.Split(stacktrace, "\n")
	lines := strings.Split(code, "\n")
	result = append(result, stlines[0])

	for i := 0; i < len(stlines); i++ {
		stline := stlines[i]
		result = append(result, stline)

		snum := reNumber.FindString(stline)
		if snum != "" {
			num, _ := strconv.Atoi(snum)
			for i, line := range lines {
				line = fmt.Sprintf("%3d %s", i+1, line)
				if i+1 > num-3 && i+1 < num+3 {
					result = append(result, line)
				}
			}
		}
	}

	return strings.Join(result, "\n")
}
