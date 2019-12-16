package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

func getToken(account string) string {
	passkey := getPassKey(account)
	return base64.StdEncoding.EncodeToString([]byte(account + ":" + passkey))
}

func accountFromToken(token string) (string, bool) {
	rtoken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", false
	}
	spl := strings.Split(string(rtoken), ":")
	if len(spl) != 2 {
		return "", false
	}

	account := spl[0]
	passkey := spl[1]
	return account, passkey == getPassKey(account)
}

func getPassKey(account string) string {
	hash := sha256.Sum256([]byte(account + "~" + s.SecretKey))
	return hex.EncodeToString(hash[:])
}
