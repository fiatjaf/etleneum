package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func getInvoice(label, desc string, msats int) (string, bool, error) {
	res, err := ln.Call("listinvoices", label)
	if err != nil {
		return "", false, err
	}

	switch res.Get("invoices.0.status").String() {
	case "paid":
		return "", true, nil
	case "unpaid":
		bolt11 := res.Get("invoices.0.bolt11").String()
		return bolt11, false, nil
	case "expired":
		_, err := ln.Call("delinvoice", label, "expired")
		if err != nil {
			return "", false, err
		}
		res, err := ln.CallWithCustomTimeout("invoice",
			7*time.Second,
			strconv.Itoa(msats), label, desc, "36000")
		if err != nil {
			return "", false, err
		}
		bolt11 := res.Get("bolt11").String()
		return bolt11, false, nil
	case "":
		res, err := ln.CallWithCustomTimeout("invoice", 7*time.Second, strconv.Itoa(msats), label, desc, "36000")
		if err != nil {
			return "", false, err
		}
		bolt11 := res.Get("bolt11").String()
		return bolt11, false, nil
	default:
		log.Warn().Str("label", label).Str("r", res.String()).
			Msg("what's up with this invoice?")
		return "", false, errors.New("bad wrong invoice")
	}
}

func checkPayment(label string, pricemsats int) (err error) {
	if pricemsats == 0 {
		return errors.New("tried to check a payment with price zero")
	}

	res, err := ln.Call("listinvoices", label)
	if err != nil {
		return
	}

	if res.Get("invoices.0.status").String() != "paid" {
		err = errors.New("invoice not paid")
		return
	}

	totalpaid := int(res.Get("invoices.0.msatoshi_received").Int())
	if totalpaid < pricemsats {
		err = fmt.Errorf("paid %d, needed %d", totalpaid, pricemsats)
		return
	}

	return nil
}

var reNumber = regexp.MustCompile("\\d+")

func stackTraceWithCode(stacktrace string, code string) string {
	var result []string

	stlines := strings.Split(stacktrace, "\n")
	lines := strings.Split(code, "\n")
	result = append(result, stlines[0])

	for i := 1; i < len(stlines); i++ {
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

func luaErrorType(apierr *lua.ApiError) string {
	switch apierr.Type {
	case lua.ApiErrorSyntax:
		return "ApiErrorSyntax"
	case lua.ApiErrorFile:
		return "ApiErrorFile"
	case lua.ApiErrorRun:
		return "ApiErrorRun"
	case lua.ApiErrorError:
		return "ApiErrorError"
	case lua.ApiErrorPanic:
		return "ApiErrorPanic"
	default:
		return "unknown"
	}
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(Result{
		Ok:    false,
		Error: message,
	})
}
