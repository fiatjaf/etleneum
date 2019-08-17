package main

import (
	"strings"

	"github.com/tidwall/gjson"
)

func handleInvoicePaid(res gjson.Result) {
	index := res.Get("pay_index").Int()
	rds.Set("lastinvoiceindex", index, 0)

	label := res.Get("label").String()
	msats := res.Get("msatoshi_received").Int()

	log.Info().Str("label", label).Int64("msats", msats).
		Str("desc", res.Get("description").String()).
		Msg("got payment")

	switch {
	// contract refill
	case strings.HasPrefix(label, s.ServiceId+".refill."):
		// get the contract from the label
		ctid := strings.Split(label, ".")[2]
		_, err = pg.Exec(
			`UPDATE contracts SET refilled = refilled + $2 WHERE id = $1`,
			ctid, msats)
		log.Info().Err(err).Str("ctid", ctid).Int64("msats", msats).Msg("contract refill")

	default:
		// for now we won't handle this here.
		log.Debug().Str("label", label).Msg("not handling payment.")
	}
}
