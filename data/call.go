package data

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Call struct {
	Id         string          `json:"id"` // used in the invoice label
	Time       time.Time       `json:"time"`
	ContractId string          `json:"contract_id"`
	Method     string          `json:"method"`
	Payload    json.RawMessage `json:"payload"`
	Msatoshi   int64           `json:"msatoshi"`       // msats to be added to the contract
	Cost       int64           `json:"cost,omitempty"` // msats to be paid to the platform
	Caller     string          `json:"caller"`
}

type Transfer struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Msatoshi int64  `json:"msatoshi"`
}

func GetCall(contract string, id string) (call *Call, err error) {
	path := filepath.Join(DatabasePath, "contracts", contract, "calls", id[1:2], id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	call = &Call{}

	if err := readJSON(filepath.Join(path, "payload.json"), &call.Payload); err != nil {
		return nil, err
	}

	if callerb, err := ioutil.ReadFile(filepath.Join(path, "caller.txt")); err == nil {
		call.Caller = string(callerb)
	}

	if methodb, err := ioutil.ReadFile(filepath.Join(path, "method.txt")); err != nil {
		return nil, err
	} else {
		call.Method = string(methodb)
	}

	if csv, err := ioutil.ReadFile(filepath.Join(path, "transfers.csv")); err == nil {
		for _, line := range strings.Split(string(csv), "\n") {
			spl := strings.Split(line, ",")
			if spl[2] == contract {
				cost, _ := strconv.ParseInt(spl[1], 10, 64)
				call.Msatoshi += cost
			}
		}
	}

	call.Time = gitGetLastCommitFileTimestamp(filepath.Join(path, "method.txt"))
	call.Id = filepath.Base(path)
	call.ContractId = contract

	return call, nil
}

func SaveCall(call *Call) error {
	path := filepath.Join(DatabasePath,
		"contracts", call.ContractId,
		"calls", call.Id[1:2], call.Id,
	)

	err := os.MkdirAll(path, 0o700)
	if err != nil {
		return err
	}

	if err := writeJSON(filepath.Join(path, "payload.json"), call.Payload); err != nil {
		return err
	}
	if err := writeFile(
		filepath.Join(path, "method.txt"),
		[]byte(call.Method),
	); err != nil {
		return err
	}
	if call.Caller != "" {
		if err := writeFile(
			filepath.Join(path, "caller.txt"),
			[]byte(call.Caller),
		); err != nil {
			return err
		}
	}

	return nil
}

func SaveTransfers(call *Call, transfers []Transfer) error {
	csv := make([]string, len(transfers))
	for i, transfer := range transfers {
		csv[i] = fmt.Sprintf("%s,%d,%s", transfer.From, transfer.Msatoshi, transfer.To)
	}

	return writeFile(
		filepath.Join(DatabasePath,
			"contracts", call.ContractId,
			"calls", call.Id[1:2], call.Id,
			"transfers.csv",
		),
		[]byte(strings.Join(csv, "\n")),
	)
}
