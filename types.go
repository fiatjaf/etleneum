package main

import "github.com/jmoiron/sqlx/types"

type Contract struct {
	Id    int    `db:"id"`
	Code  string `db:"code"`
	State types.JSONText
}
