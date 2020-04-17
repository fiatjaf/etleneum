package types

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

type Contract struct {
	Id        string         `db:"id" json:"id"` // used in the invoice label
	Code      string         `db:"code" json:"code,omitempty"`
	Name      string         `db:"name" json:"name"`
	Readme    string         `db:"readme" json:"readme"`
	State     types.JSONText `db:"state" json:"state,omitempty"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`

	Funds  int64 `db:"funds" json:"funds"` // contract balance in msats
	NCalls int   `db:"ncalls" json:"ncalls,omitempty"`
}

const CONTRACTFIELDS = "id, code, name, readme, state, created_at"

type Call struct {
	Id         string         `db:"id" json:"id"` // used in the invoice label
	Time       time.Time      `db:"time" json:"time"`
	ContractId string         `db:"contract_id" json:"contract_id"`
	Method     string         `db:"method" json:"method"`
	Payload    types.JSONText `db:"payload" json:"payload"`
	Msatoshi   int64          `db:"msatoshi" json:"msatoshi"` // msats to be added to the contract
	Cost       int64          `db:"cost" json:"cost"`         // msats to be paid to the platform
	Caller     string         `db:"caller" json:"caller"`
	Diff       string         `db:"diff" json:"diff"`
	Transfers  types.JSONText `db:"transfers" json:"transfers"`
}

const CALLFIELDS = "id, time, contract_id, method, payload, msatoshi, cost, coalesce(caller, '') AS caller, transfers(id, contract_id) AS transfers"

type Account struct {
	Id      string `db:"id" json:"id"`
	Balance int64  `db:"balance" json:"balance"`
}

const ACCOUNTFIELDS = "id, accounts.balance"

type StuffBeingCreated struct {
	Id      string `json:"id"`
	Invoice string `json:"invoice"`
}
