package main

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

type Contract struct {
	Id     string         `db:"id" json:"id"`
	Code   string         `db:"code" json:"code"`
	Name   string         `db:"name" json:"name"`
	Readme string         `db:"readme" json:"readme"`
	State  types.JSONText `db:"state" json:"state"`
	Funds  int            `db:"funds" json:"funds"`
	Bolt11 string         `db:"-" json:"invoice,omitempty"`
}

func (c *Contract) getInvoice() error {
	label := s.ServiceId + "." + c.Id
	desc := "etleneum contract __init__ [" + c.Id + "]"
	msats := s.InitCostSatoshis * 1000
	bolt11, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	return err
}

type Call struct {
	Id         string         `db:"id" json:"id"`
	Hash       string         `db:"hash" json:"hash"`
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Satoshis   int            `db:"satoshis" json:"satoshis"`
	Cost       int            `db:"cost" json:"cost"`
	Bolt11     string         `db:"-" json:"invoice,omitempty"`
}

func (c *Call) calcCosts() {
	c.Cost = 1000
	c.Cost += len(c.Payload)
}

func (c *Call) getInvoice() error {
	label := s.ServiceId + "." + c.ContractId + "." + c.Id
	desc := "etleneum contract call [" + c.ContractId + "][" + c.Id + "]"
	msats := c.Cost + 1000*c.Satoshis
	bolt11, err := getInvoice(label, desc, msats)
	c.Bolt11 = bolt11
	return err
}
