package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/aarzilli/golua/lua"
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

func checkContractCode(code string) (ok bool) {
	if strings.Index(code, "function __init__") == -1 {
		return false
	}

	L := lua.NewState()
	defer L.Close()

	lerr := L.LoadString(code)
	if lerr != 0 {
		return false
	}

	return true
}

var wordMatcher *regexp.Regexp = regexp.MustCompile("\b\\w+\b")

func getContractCost(ct types.Contract) int {
	words := len(wordMatcher.FindAllString(ct.Code, -1))
	return 1000*s.InitialContractCostSatoshis + 1000*words
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
