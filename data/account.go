package data

import (
	"encoding/json"
	"path/filepath"
)

func GetAccountBalance(key string) (msatoshi int64) {
	readJSON(filepath.Join(DatabasePath, "accounts", key, "balance.json"), msatoshi)
	return msatoshi
}

func SaveAccountBalance(key string, msatoshi int64) error {
	balanceJSON, _ := json.Marshal(msatoshi)
	return writeFile(
		filepath.Join(DatabasePath, "accounts", key, "balance.json"),
		balanceJSON,
	)
}
