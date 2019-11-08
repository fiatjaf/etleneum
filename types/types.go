package types

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

type Contract struct {
	Id        string         `db:"id" json:"id"` // used in the invoice label
	Code      string         `db:"code" json:"code"`
	Name      string         `db:"name" json:"name"`
	Readme    string         `db:"readme" json:"readme"`
	State     types.JSONText `db:"state" json:"state"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`

	Funds       int    `db:"funds" json:"funds"` // contract balance in msats
	NCalls      int    `db:"ncalls" json:"ncalls,omitempty"`
	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid *bool  `db:"-" json:"invoice_paid,omitempty"`
}

const CONTRACTFIELDS = "id, code, name, readme, state, created_at"

type Call struct {
	Id         string         `db:"id" json:"id"` // used in the invoice label
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Msatoshi   int            `db:"msatoshi" json:"msatoshi"` // msats to be added to the contract
	Cost       int            `db:"cost" json:"cost"`         // msats to be paid to the platform
	Caller     string         `db:"caller" json:"caller"`

	Bolt11      string `db:"-" json:"invoice,omitempty"`
	InvoicePaid *bool  `db:"-" json:"invoice_paid,omitempty"`
}

const CALLFIELDS = "id, time, contract_id, method, payload, msatoshi, cost, coalesce(caller, '') AS caller"

type Account struct {
	Id      string `db:"id" json:"id"`
	Balance int    `db:"balance" json:"balance"`
}

const ACCOUNTFIELDS = "id, accounts.balance"

type ContractEvent struct {
	Kind     string         `db:"kind" json:"kind"`
	Call     string         `db:"call" json:"call"`
	Time     time.Time      `db:"time" json:"time"`
	Msatoshi int            `db:"msatoshi" json:"msatoshi"`
	Data     types.JSONText `db:"data" json:"data"`
}

const CONTRACTEVENTFIELDS = "kind, call, time, msatoshi, data"

type Refund struct {
	PaymentHash string    `db:"payment_hash" json:"hash"`
	Time        time.Time `db:"time" json:"time"`
	Msatoshi    int       `db:"msatoshi" json:"msatoshi"`
	Claimed     bool      `db:"claimed" json:"-"`
	Fulfilled   bool      `db:"fulfilled" json:"-"`
}

const REFUNDFIELDS = "payment_hash, time, msatoshi"
