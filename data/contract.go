package data

import (
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	functionRe = regexp.MustCompile(`^function +([^_][\w_]+) *\(`)
	paramRe    = regexp.MustCompile(`\bcall.payload\.([\w_]+)`)
	authRe     = regexp.MustCompile(`\b(account.send|account.id|account.get_balance)\b`)
	endRe      = regexp.MustCompile(`^end\b`)
)

type Contract struct {
	Id      string          `json:"id"` // used in the invoice label
	Code    string          `json:"code,omitempty"`
	Name    string          `json:"name"`
	Readme  string          `json:"readme"`
	State   json.RawMessage `json:"state,omitempty"`
	Funds   int64           `json:"funds"` // contract balance in msats
	Methods []Method        `json:"methods"`
}

type Method struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
	Auth   bool     `json:"auth"`
}

func ListContracts() (contracts []Contract, err error) {
	contractsPath := filepath.Join(DatabasePath, "contracts")
	err = filepath.WalkDir(contractsPath,
		func(path string, info fs.DirEntry, err error) error {
			if path == contractsPath {
				return nil
			}

			if err != nil {
				log.Error().Err(err).Str("path", path).
					Msg("error reading contract dir")
				return err
			}

			nameb, _ := ioutil.ReadFile(filepath.Join(path, "name.txt"))
			readmeb, _ := ioutil.ReadFile(filepath.Join(path, "README.md"))
			var funds int64
			readJSON(filepath.Join(path, "funds.json"), &funds)

			contracts = append(contracts, Contract{
				Id:     filepath.Base(path),
				Name:   string(nameb),
				Readme: string(readmeb),
				Funds:  funds,
			})

			if info.IsDir() {
				return fs.SkipDir
			} else {
				return nil
			}
		},
	)
	if err != nil {
		return nil, err
	}

	if contracts == nil {
		contracts = make([]Contract, 0)
	}
	return contracts, nil
}

func GetContract(id string) (contract *Contract, err error) {
	path := filepath.Join(DatabasePath, "contracts", id)
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}

	nameb, err := ioutil.ReadFile(filepath.Join(path, "name.txt"))
	if err != nil {
		return nil, err
	}
	readmeb, err := ioutil.ReadFile(filepath.Join(path, "README.md"))
	if err != nil {
		return nil, err
	}
	codeb, err := ioutil.ReadFile(filepath.Join(path, "contract.lua"))
	if err != nil {
		return nil, err
	}
	var funds int64
	err = readJSON(filepath.Join(path, "funds.json"), &funds)
	if err != nil {
		return nil, err
	}
	var state json.RawMessage
	err = readJSON(filepath.Join(path, "state.json"), &state)
	if err != nil {
		return nil, err
	}

	contract = &Contract{
		Id:     filepath.Base(path),
		Name:   string(nameb),
		Readme: string(readmeb),
		Code:   string(codeb),
		State:  state,
		Funds:  funds,
	}
	parseContractCode(contract)

	return contract, nil
}

func CreateContract(
	id string,
	name string,
	readme string,
	code string,
) error {
	path := filepath.Join(DatabasePath, "contracts", id)
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(path, "name.txt"), []byte(name)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(path, "README.md"), []byte(readme)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(path, "contract.lua"), []byte(code)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(path, "state.json"), []byte("{}")); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(path, "funds.json"), []byte("0")); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(path, "calls"), 0700); err != nil {
		return err
	}

	return nil
}

func SaveContractState(id string, state json.RawMessage) error {
	return writeJSON(
		filepath.Join(DatabasePath, "contracts", id, "state.json"),
		state,
	)
}

func SaveContractFunds(id string, msatoshi int64) error {
	return writeJSON(
		filepath.Join(DatabasePath, "contracts", id, "funds.json"),
		msatoshi,
	)
}

func DeleteContract(id string) error {
	path := filepath.Join(DatabasePath, "contracts", id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	Start()
	if err := os.RemoveAll(path); err != nil {
		Abort()
		return err
	}

	if err := gitAdd(path); err != nil {
		Abort()
		return err
	}

	Finish("contract " + id + " deleted.")
	return nil
}

func parseContractCode(ct *Contract) {
	lines := strings.Split(ct.Code, "\n")

	var currentMethod *Method
	var params map[string]bool
	for _, line := range lines {
		if matches := functionRe.FindStringSubmatch(line); len(matches) == 2 {
			currentMethod = &Method{
				Name:   matches[1],
				Params: make([]string, 0, 3),
			}
			params = make(map[string]bool)
		}

		if currentMethod == nil {
			continue
		}

		if authRe.MatchString(line) {
			currentMethod.Auth = true
		}

		matches := paramRe.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			params[match[1]] = true
		}

		if endRe.MatchString(line) {
			for param, _ := range params {
				currentMethod.Params = append(currentMethod.Params, param)
			}

			ct.Methods = append(ct.Methods, *currentMethod)
			currentMethod = nil
			params = nil
		}
	}
}
