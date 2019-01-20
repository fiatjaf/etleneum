package main

import (
	"errors"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx/types"
)

type Contract struct {
	Id     int            `db:"id" json:"-"`
	Hashid string         `db:"-" json:"id"`
	Code   string         `db:"code" json:"code"`
	Name   string         `db:"name" json:"name"`
	Readme string         `db:"readme" json:"readme"`
	State  types.JSONText `db:"state" json:"state"`
	Bolt11 string         `db:"-" json:"invoice,omitempty"`
}

func (c *Contract) makeHashid() error {
	if c.Id == 0 {
		return errors.New("no id")
	}
	e, err := h.Encode([]int{c.Id})
	if err != nil {
		return err
	}
	c.Hashid = e
	return nil
}

func (c *Contract) makeId() error {
	if c.Hashid == "" {
		return errors.New("no id")
	}
	d, err := h.DecodeWithError(c.Hashid)
	if err != nil {
		return err
	}
	if len(d) != 1 {
		return errors.New("wrong value decoded")
	}
	c.Id = d[0]
	return nil
}

func (c *Contract) getInvoice() error {
	label := s.ServiceId + "." + c.Hashid
	res, err := ln.Call("listinvoices", label)
	if err != nil {
		return err
	}

	switch res.Get("invoices.0.status").String() {
	case "paid":
		return nil
	case "unpaid":
		c.Bolt11 = res.Get("invoices.0.bolt11").String()
		return nil
	case "expired":
		_, err := ln.Call("delinvoice", label, "expired")
		if err != nil {
			return err
		}
		res, err := ln.Call("invoice", strconv.Itoa(s.InitCostSatoshis*1000),
			label, "etleneum initial contract activation.")
		if err != nil {
			return err
		}
		c.Bolt11 = res.Get("bolt11").String()
		return nil
	case "":
		res, err := ln.Call("invoice", strconv.Itoa(s.InitCostSatoshis*1000),
			label, "etleneum initial contract activation.")
		if err != nil {
			return err
		}
		c.Bolt11 = res.Get("bolt11").String()
		return nil
	default:
		log.Warn().Str("label", label).Str("r", res.String()).
			Msg("what's up with this invoice?")
		return errors.New("wrong invoice")
	}
}

type Call struct {
	Hash       string         `db:"hash" json:"hash"`
	Label      string         `db:"label" json:"label"`
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Satoshis   int            `db:"satoshis" json:"satoshis"`
}
