package data

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

func GetAccountBalance(key string) (msatoshi int64) {
	readJSON(filepath.Join(DatabasePath, "accounts", key, "balance.json"), msatoshi)
	return msatoshi
}

func SaveAccountBalance(key string, msatoshi int64) {
	balanceJSON, _ := json.Marshal(msatoshi)
	ioutil.WriteFile(
		filepath.Join(DatabasePath, "accounts", key, "balance.json"),
		balanceJSON,
		0644,
	)
}
