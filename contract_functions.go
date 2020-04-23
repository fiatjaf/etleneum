package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/aarzilli/golua/lua"
	"github.com/fiatjaf/etleneum/types"
)

func contractFromRedis(ctid string) (ct *types.Contract, err error) {
	var jct []byte
	ct = &types.Contract{}

	jct, err = rds.Get("contract:" + ctid).Bytes()
	if err != nil {
		return
	}

	err = json.Unmarshal(jct, ct)
	if err != nil {
		return
	}

	return
}

var (
	functionRe = regexp.MustCompile(`^function +([^_][\w_]+) *\(`)
	paramRe    = regexp.MustCompile(`\bcall.payload\.([\w_]+)`)
	authRe     = regexp.MustCompile(`\b(account.send|account.id|account.get_balance)\b`)
	endRe      = regexp.MustCompile(`^end\b`)
)

func parseContractCode(ct *types.Contract) {
	lines := strings.Split(ct.Code, "\n")

	var currentMethod *types.Method
	var params map[string]bool
	for _, line := range lines {
		if matches := functionRe.FindStringSubmatch(line); len(matches) == 2 {
			currentMethod = &types.Method{
				Name:   matches[1],
				Params: make([]string, 0, 3),
			}
			params = make(map[string]bool)
		}

		if currentMethod == nil {
			continue
		}

		if authRe.MatchString(line) {
			currentMethod.Auth = true
		}

		matches := paramRe.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			params[match[1]] = true
		}

		if endRe.MatchString(line) {
			for param, _ := range params {
				currentMethod.Params = append(currentMethod.Params, param)
			}

			ct.Methods = append(ct.Methods, *currentMethod)
			currentMethod = nil
			params = nil
		}
	}
}

func checkContractCode(code string) (ok bool) {
	if strings.Index(code, "function __init__") == -1 {
		return false
	}

	L := lua.NewState()
	defer L.Close()

	lerr := L.LoadString(code)
	if lerr != 0 {
		return false
	}

	return true
}

func getContractCost(ct types.Contract) int64 {
	words := int64(len(wordMatcher.FindAllString(ct.Code, -1)))
	return 1000*s.InitialContractCostSatoshis + 1000*words
}

func saveContractOnRedis(ct types.Contract) (jct []byte, err error) {
	jct, err = json.Marshal(ct)
	if err != nil {
		return
	}

	err = rds.Set("contract:"+ct.Id, jct, time.Hour*20).Err()
	if err != nil {
		return
	}

	return
}
