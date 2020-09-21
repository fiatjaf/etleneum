package runlua

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	decodepay "github.com/fiatjaf/ln-decodepay"
)

func make_lua_http(makeRequest func(*http.Request) (*http.Response, error)) (
	lua_http_gettext func(string, ...map[string]interface{}) (string, error),
	lua_http_getjson func(string, ...map[string]interface{}) (interface{}, error),
	lua_http_postjson func(string, interface{}, ...map[string]interface{}) (interface{}, error),
	calls_p *int,
) {
	calls := 0
	calls_p = &calls

	http_call := func(method, url string, body interface{}, headers ...map[string]interface{}) (b []byte, err error) {
		log.Debug().Str("method", method).Interface("body", body).Str("url", url).Msg("http call from contract")

		bodyjson := new(bytes.Buffer)
		if body != nil {
			err = json.NewEncoder(bodyjson).Encode(body)
			if err != nil {
				log.Warn().Err(err).Msg("http: failed to encode body")
				return
			}
			headers = append([]map[string]interface{}{{"Content-Type": "application/json"}}, headers...)
		}

		req, err := http.NewRequest(method, url, bodyjson)
		if err != nil {
			log.Warn().Err(err).Msg("http: failed to create request")
			return
		}
		defer req.Body.Close()

		for _, headermap := range headers {
			for k, v := range headermap {
				if sv, ok := v.(string); ok {
					req.Header.Set(k, sv)
				}
			}
		}

		resp, err := makeRequest(req)
		if err != nil {
			log.Warn().Err(err).Msg("http: failed to make request")
			return
		}

		if resp.StatusCode >= 300 {
			log.Debug().Err(err).Int("code", resp.StatusCode).Msg("http: got bad status")
			err = errors.New("response status code: " + strconv.Itoa(resp.StatusCode))
			return
		}

		b, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Warn().Err(err).Msg("http: failed to read body")
			return
		}

		return b, nil
	}

	lua_http_gettext = func(url string, headers ...map[string]interface{}) (t string, err error) {
		respbytes, err := http_call("GET", url, nil, headers...)
		if err != nil {
			return "", err
		}
		return string(respbytes), nil
	}

	lua_http_getjson = func(url string, headers ...map[string]interface{}) (j interface{}, err error) {
		respbytes, err := http_call("GET", url, nil, headers...)
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

	lua_http_postjson = func(url string, body interface{}, headers ...map[string]interface{}) (j interface{}, err error) {
		respbytes, err := http_call("POST", url, body, headers...)
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

func lua_parse_bolt11(bolt11 string) (map[string]interface{}, error) {
	inv, err := decodepay.Decodepay(bolt11)
	if err != nil {
		return nil, err
	}
	jinv, _ := json.Marshal(inv)
	minv := make(map[string]interface{})
	json.Unmarshal(jinv, &minv)
	return minv, nil
}

func lua_check_btc_address(address string) error {
	_, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	return err
}
