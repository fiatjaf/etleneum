package main

import (
	"encoding/json"
	"errors"
	"fmt"
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

func checkPayment(label string, pricemsats int) (msats int, err error) {
	if pricemsats == 0 {
		return 0, errors.New("tried to check a payment with price zero")
	}

	if s.Development {
		return pricemsats, nil
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

	return int(res.Get("invoices.0.msatoshi_received").Int()), nil
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(Result{
		Ok:    false,
		Error: message,
	})
}
