package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func handleDecodeScid(w http.ResponseWriter, r *http.Request) {
	scid := mux.Vars(r)["scid"]

	uscid, err := decodeShortChannelId(scid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	callid, ok := parseShortChannelId(uscid)
	if !ok {
		http.Error(w, "couldn't parse, not a call id.", 400)
		return
	}

	returnCallDetails(w, callid)
}

func handleCallDetails(w http.ResponseWriter, r *http.Request) {
	callid := mux.Vars(r)["callid"]
	returnCallDetails(w, callid)
}

func returnCallDetails(w http.ResponseWriter, callid string) {
	scid := makeShortChannelId(callid)
	preimage := makePreimage(callid)
	preimageHex := hex.EncodeToString(preimage)
	hash := sha256.Sum256(preimage)

	// secret data only shown for calls already paid
	var exists bool
	pg.Get(&exists, `SELECT 1 FROM calls WHERE id = $1`, callid)
	if !exists {
		preimageHex = "<secret as this call isn't paid yet>"
	}

	fmt.Fprintf(w, `
call: %s
short_channel_id: %s
preimage: %s
hash: %s

    `, callid,
		encodeShortChannelId(scid),
		preimageHex,
		hex.EncodeToString(hash[:]),
	)
}
