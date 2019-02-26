package types

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

type Contract struct {
	Id           string         `db:"id" json:"id"` // used in the invoice label
	Code         string         `db:"code" json:"code"`
	Name         string         `db:"name" json:"name"`
	Readme       string         `db:"readme" json:"readme"`
	State        types.JSONText `db:"state" json:"state"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
	StorageCosts int            `db:"storage_costs" json:"storage_costs"` // sum of all daily storage costs, in msats
	Refilled     int            `db:"refilled" json:"refilled"`           // msats refilled without use of a normal call

	Funds       int    `db:"funds" json:"funds"` // contract balance in msats
	NCalls      int    `db:"ncalls" json:"ncalls,omitempty"`
	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid *bool  `db:"-" json:"invoice_paid,omitempty"`
}

type Call struct {
	Id         string         `db:"id" json:"id"` // used in the invoice label
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Satoshis   int            `db:"satoshis" json:"satoshis"` // sats to be added to the contract
	Cost       int            `db:"cost" json:"cost"`         // msats to be paid to the platform
	Paid       int            `db:"paid" json:"paid"`         // msats sum of payments done by this contract

	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid *bool  `db:"-" json:"invoice_paid,omitempty"`
}
