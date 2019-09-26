package runlua

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	openpgp "golang.org/x/crypto/openpgp"
	openpgperrors "golang.org/x/crypto/openpgp/errors"
)

func make_lua_http(makeRequest func(*http.Request) (*http.Response, error)) (
	lua_http_gettext func(string, ...map[string]interface{}) (string, error),
	lua_http_getjson func(string, ...map[string]interface{}) (interface{}, error),
	calls_p *int,
) {
	calls := 0
	calls_p = &calls

	http_get := func(url string, headers ...map[string]interface{}) (b []byte, err error) {
		log.Debug().Str("url", url).Msg("http call from contract")

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return
		}

		for _, headermap := range headers {
			for k, v := range headermap {
				if sv, ok := v.(string); ok {
					req.Header.Set(k, sv)
				}
			}
		}

		resp, err := makeRequest(req)
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

	lua_http_gettext = func(url string, headers ...map[string]interface{}) (t string, err error) {
		respbytes, err := http_get(url, headers...)
		if err != nil {
			return "", err
		}
		return string(respbytes), nil
	}

	lua_http_getjson = func(url string, headers ...map[string]interface{}) (j interface{}, err error) {
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

func lua_keybase_verify_signature(username, text, sig string) (ok bool, err error) {
	resp, err := http.Get("https://keybase.io/" + username + "/pgp_keys.asc")
	if err != nil {
		return false, err
	}
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("keybase returned status code %d", resp.StatusCode)
	}

	keyring, err := openpgp.ReadArmoredKeyRing(resp.Body)
	if err != nil {
		return false, err
	}

	verification_target := strings.NewReader(text)
	signature := strings.NewReader(sig)

	_, err = openpgp.CheckArmoredDetachedSignature(keyring, verification_target, signature)
	if err != nil {
		if _, ok := err.(openpgperrors.SignatureError); ok {
			// this means the signature is wrong and not some kind of operational error
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func lua_keybase_lookup(provider, name string) (username string, err error) {
	params := url.Values{}
	params.Set("fields", "basics")
	params.Set(provider, name)
	url := "https://keybase.io/_/api/1.0/user/lookup.json"
	resp, err := http.Get(url + "?" + params.Encode())
	if err != nil {
		return "", err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	gjson.GetBytes(b, "them").ForEach(func(_, match gjson.Result) bool {
		username = match.Get("basics.username").String()
		return false
	})

	return username, nil
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
