package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type AccountMetadata struct {
	BalanceNotify string `json:"balanceNotify"`
}

func GetAccountBalance(key string) (msatoshi int64) {
	if err := readJSON(
		filepath.Join(DatabasePath, "accounts", key, "balance.json"),
		&msatoshi,
	); err != nil {
		log.Warn().Err(err).Str("account", key).Msg("error reading balance.json")
		return 0
	}
	return msatoshi
}

func SaveAccountBalance(key string, msatoshi int64) error {
	path := filepath.Join(DatabasePath, "accounts", key)

	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}

	balanceJSON, _ := json.Marshal(msatoshi)
	return writeFile(
		filepath.Join(path, "balance.json"),
		balanceJSON,
	)
}

func CheckBalanceAddWithdrawal(key string, amount int64, bolt11, hash string) error {
	Start()

	balance := GetAccountBalance(key)
	reserveFee := int64(float64(amount) * 0.007)

	if err := writeFile(
		filepath.Join(DatabasePath, "accounts", key, "withdraw_"+hash+".txt"),
		[]byte(bolt11),
	); err != nil {
		Abort()
		return err
	}

	newBalance := balance - amount - reserveFee
	if newBalance < 0 {
		Abort()
		return fmt.Errorf("balance would go below zero: %d", newBalance)
	}

	if err := SaveAccountBalance(key, newBalance); err != nil {
		Abort()
		return err
	}

	Finish(fmt.Sprintf("account %s has requested a %d withdraw.", key, amount))
	return nil
}

func FulfillWithdraw(key string, amount int64, actualFee int64, hash string) error {
	Start()

	balance := GetAccountBalance(key)
	reserveFee := int64(float64(amount) * 0.007)

	if err := os.Remove(
		filepath.Join(DatabasePath, "accounts", key, "withdraw_"+hash+".txt"),
	); err != nil {
		Abort()
		return err
	}

	if err := SaveAccountBalance(key, balance+reserveFee-actualFee); err != nil {
		Abort()
		return err
	}

	Finish(fmt.Sprintf("withdraw %s has succeeded.", hash))
	return nil
}

func CancelWithdraw(key string, amount int64, hash string) error {
	Start()

	balance := GetAccountBalance(key)
	reserveFee := int64(float64(amount) * 0.007)

	if err := os.Remove(
		filepath.Join(DatabasePath, "accounts", key, "withdraw_"+hash+".txt"),
	); err != nil {
		Abort()
		return err
	}

	if err := SaveAccountBalance(key, balance+amount+reserveFee); err != nil {
		Abort()
		return err
	}

	Finish(fmt.Sprintf("withdraw %s has failed.", hash))
	return nil
}

func GetAccountMetadata(key string) (am AccountMetadata) {
	readJSON(
		filepath.Join(DatabasePath, "accounts", key, "metadata.json"),
		&am,
	)
	return am
}

func UpdateAccountMetadata(key string, mod func(am *AccountMetadata)) error {
	Start()

	am := GetAccountMetadata(key)

	mod(&am)

	err := writeJSON(
		filepath.Join(DatabasePath, "accounts", key, "metadata.json"),
		am,
	)
	if err != nil {
		Abort()
		return err
	}

	Finish(fmt.Sprintf("account %s metadata was updated.", key))
	return nil
}
