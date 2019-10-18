package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var wordMatcher *regexp.Regexp = regexp.MustCompile(`\b\w+\b`)

type Result struct {
	Ok    bool        `json:"ok"`
	Value interface{} `json:"value"`
	Error string      `json:"error,omitempty"`
}

func getInvoice(label, desc string, msats int) (string, bool, error) {
	if s.FreeMode {
		// return a bogus invoice
		return "lnbcrt1231230p1pwccq4app53nrqyuwmhkcsqqq8qnqvka0njqt0q0w9ujjlu565yumcgjya7m7qdp8vakx7cnpdss8wctjd45kueeqd9ejqcfqdphkz7qxqgzay8dellcqp2r34dm702mtt9luaeuqfza47ltalrwk8jrwalwf5ncrkgm6v6kmm3cuwuhyhtkpyzzmxun8qz9qtx6hvwfltqnd6wvpkch2u3acculmqpk4d20k", false, nil
	}

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
		res, err := ln.CallWithCustomTimeout(7*time.Second,
			"invoice", strconv.Itoa(msats), label, desc, "36000")
		if err != nil {
			return "", false, err
		}
		bolt11 := res.Get("bolt11").String()
		return bolt11, false, nil
	case "":
		res, err := ln.CallWithCustomTimeout(7*time.Second,
			"invoice", strconv.Itoa(msats), label, desc, "36000")
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

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(Result{
		Ok:    false,
		Error: message,
	})
}
