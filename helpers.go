package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/yudai/gojsondiff"
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

func diffDeltaOneliner(prefix string, idelta gojsondiff.Delta) (lines []string) {
	key := prefix
	if key != "" {
		key += "."
	}

	switch pdelta := idelta.(type) {
	case gojsondiff.PreDelta:
		switch delta := pdelta.(type) {
		case *gojsondiff.Moved:
			key = key + delta.PrePosition().String()
			lines = append(lines, fmt.Sprintf("- %s", key))
		case *gojsondiff.Deleted:
			key = key + delta.PrePosition().String()
			lines = append(lines, fmt.Sprintf("- %s", key[:len(key)-1]))
		}
	case gojsondiff.PostDelta:
		switch delta := pdelta.(type) {
		case *gojsondiff.TextDiff:
			key = key + delta.PostPosition().String()
			lines = append(lines, fmt.Sprintf("= %s %v", key, delta.NewValue))
		case *gojsondiff.Modified:
			key = key + delta.PostPosition().String()
			lines = append(lines, fmt.Sprintf("= %s %v", key, delta.NewValue))
		case *gojsondiff.Added:
			key = key + delta.PostPosition().String()
			lines = append(lines, fmt.Sprintf("+ %s %v", key, delta.Value))
		case *gojsondiff.Object:
			key = key + delta.PostPosition().String()
			for _, nextdelta := range delta.Deltas {
				lines = append(lines, diffDeltaOneliner(key, nextdelta)...)
			}
		case *gojsondiff.Array:
			key = key + delta.PostPosition().String()
			for _, nextdelta := range delta.Deltas {
				lines = append(lines, diffDeltaOneliner(key, nextdelta)...)
			}
		case *gojsondiff.Moved:
			key = key + delta.PostPosition().String()
			lines = append(lines, fmt.Sprintf("+ %s %v", key, delta.Value))
			if delta.Delta != nil {
				if d, ok := delta.Delta.(gojsondiff.Delta); ok {
					lines = append(lines, diffDeltaOneliner(key, d)...)
				}
			}
		}
	}

	return
}
