package main

import (
	"encoding/json"
	"time"

	"github.com/fiatjaf/etleneum/types"
)

func contractFromRedis(ctid string) (ct *types.Contract, err error) {
	var jct []byte
	ct = &types.Contract{}

	jct, err = rds.Get("contract:" + ctid).Bytes()
	if err != nil {
		return
	}

	err = json.Unmarshal(jct, ct)
	if err != nil {
		return
	}

	return
}

func getContractCost(ct types.Contract) int {
	return 1000*s.InitialContractCostSatoshis + 1000*len(ct.Code)
}

func getContractInvoice(ct *types.Contract) error {
	label := s.ServiceId + "." + ct.Id
	desc := s.ServiceId + " __init__ [" + ct.Id + "]"
	msats := getContractCost(*ct) + 1000*s.InitialContractFillSatoshis
	bolt11, paid, err := getInvoice(label, desc, msats)
	ct.Bolt11 = bolt11
	ct.InvoicePaid = &paid
	return err
}

func saveContractOnRedis(ct types.Contract) (jct []byte, err error) {
	jct, err = json.Marshal(ct)
	if err != nil {
		return
	}

	err = rds.Set("contract:"+ct.Id, jct, time.Hour*20).Err()
	if err != nil {
		return
	}

	return
}
