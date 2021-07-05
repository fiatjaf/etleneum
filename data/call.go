package data

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Call struct {
	Id         string          `json:"id"` // used in the invoice label
	Time       time.Time       `json:"time"`
	ContractId string          `json:"contract_id,omitempty"`
	Method     string          `json:"method"`
	Payload    json.RawMessage `json:"payload"`
	Msatoshi   int64           `json:"msatoshi"` // msats to be added to the contract
	Cost       int64           `json:"cost"`     // msats to be paid to the platform
	Caller     string          `json:"caller,omitempty"`
}

type Transfer struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Msatoshi int64  `json:"msatoshi"`
}

func GetCall(contract string, id string) (call *Call, err error) {
	path := filepath.Join(DatabasePath, "contracts", contract, "calls", id[0:1], id)
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}

	err = readJSON(filepath.Join(path, "call.json"), &call)
	if err != nil {
		return nil, err
	}
	call.Id = filepath.Base(path)

	return call, nil
}

func SaveCall(call *Call) error {
	path := filepath.Join(DatabasePath,
		"contracts", call.ContractId,
		"calls", call.Id[0:1], call.Id,
	)

	err := os.MkdirAll(path, 0700)
	if err != nil {
		return err
	}

	callJSON, err := json.Marshal(call)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(path, "call.json"), callJSON, 0644)
	if err != nil {
		return err
	}

	return nil
}

func SaveTransfers(call *Call, transfers []Transfer) {
	csv := make([]string, len(transfers))
	for i, transfer := range transfers {
		csv[i] = fmt.Sprintf("%s,%d,%s", transfer.From, transfer.Msatoshi, transfer.To)
	}

	ioutil.WriteFile(
		filepath.Join(DatabasePath,
			"contracts", call.ContractId,
			"calls", call.Id[0:1], call.Id,
			"transfers.csv",
		),
		[]byte(strings.Join(csv, "\n")),
		0644,
	)
}
