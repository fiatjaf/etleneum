package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

func getInvoice(label, desc string, msats int) (string, error) {
	res, err := ln.Call("listinvoices", label)
	if err != nil {
		return "", err
	}

	switch res.Get("invoices.0.status").String() {
	case "paid":
		return "", nil
	case "unpaid":
		bolt11 := res.Get("invoices.0.bolt11").String()
		return bolt11, nil
	case "expired":
		_, err := ln.Call("delinvoice", label, "expired")
		if err != nil {
			return "", err
		}
		res, err := ln.CallWithCustomTimeout("invoice", 7*time.Second, strconv.Itoa(msats), label, desc)
		if err != nil {
			return "", err
		}
		bolt11 := res.Get("bolt11").String()
		return bolt11, nil
	case "":
		res, err := ln.CallWithCustomTimeout("invoice", 7*time.Second, strconv.Itoa(msats), label, desc)
		if err != nil {
			return "", err
		}
		bolt11 := res.Get("bolt11").String()
		return bolt11, nil
	default:
		log.Warn().Str("label", label).Str("r", res.String()).
			Msg("what's up with this invoice?")
		return "", errors.New("bad wrong invoice")
	}
}

func checkPayment(label string, pricemsats int) (hash string, err error) {
	res, err := ln.Call("listinvoices", label)
	if err != nil {
		return
	}

	hash = res.Get("invoices.0.payment_hash").String()
	if res.Get("invoices.0.status").String() != "paid" {
		err = errors.New("invoice not paid")
		return
	}

	totalpaid := int(res.Get("invoices.0.msatoshi_received").Int())
	if totalpaid < pricemsats {
		err = fmt.Errorf("paid %d, needed %d", totalpaid, pricemsats)
		return
	}

	return hash, nil
}
