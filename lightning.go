package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/zpay32"
)

const BOGUS_INVOICE = "lnbcrt1231230p1pwccq4app53nrqyuwmhkcsqqq8qnqvka0njqt0q0w9ujjlu565yumcgjya7m7qdp8vakx7cnpdss8wctjd45kueeqd9ejqcfqdphkz7qxqgzay8dellcqp2r34dm702mtt9luaeuqfza47ltalrwk8jrwalwf5ncrkgm6v6kmm3cuwuhyhtkpyzzmxun8qz9qtx6hvwfltqnd6wvpkch2u3acculmqpk4d20k"

var BOGUS_SECRET = [32]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

func makeInvoice(
	id string, // call or contract id: pubkey, scid and preimage based on this
	desc string,
	deschash *[32]byte,
	msatoshi int64,
	cost int64, // will be added as routing fees in the last channel
) (bolt11 string, err error) {
	preimage := makePreimage(id)
	sk, _ := makeKeys(id)
	channelid := makeShortChannelId(id)

	nodeid, _ := hex.DecodeString(s.NodeId)
	ournodeid, err := btcec.ParsePubKey(nodeid, btcec.S256())
	if err != nil {
		return "", fmt.Errorf("error parsing our own nodeid: %w", err)
	}

	var addDescription func(*zpay32.Invoice)
	if deschash != nil {
		addDescription = zpay32.DescriptionHash(*deschash)
	} else {
		addDescription = zpay32.Description(desc)
	}

	invoice, err := zpay32.NewInvoice(
		&chaincfg.Params{Bech32HRPSegwit: "bc"},
		sha256.Sum256(preimage),
		time.Now(),
		zpay32.RouteHint([]zpay32.HopHint{
			zpay32.HopHint{
				NodeID:                    ournodeid,
				ChannelID:                 channelid,
				FeeBaseMSat:               uint32(cost),
				FeeProportionalMillionths: 0,
				CLTVExpiryDelta:           2,
			},
		}),
		zpay32.Amount(lnwire.MilliSatoshi(msatoshi)),
		zpay32.Expiry(time.Hour*24),
		zpay32.Features(&lnwire.FeatureVector{
			RawFeatureVector: lnwire.NewRawFeatureVector(
				lnwire.PaymentAddrOptional,
				lnwire.MPPOptional,
			),
		}),
		zpay32.PaymentAddr(BOGUS_SECRET),
		addDescription,
	)

	return invoice.Encode(zpay32.MessageSigner{
		SignCompact: func(hash []byte) ([]byte, error) {
			return btcec.SignCompact(btcec.S256(), sk, hash, true)
		},
	})
}

func makeShortChannelId(id string) uint64 {
	var scid string
	for _, letter := range []byte(id) {
		scid += strconv.FormatInt(int64(letter-40), 10)
	}

	x := len(scid) / 3
	// scid = scid[:x] + "x" + scid[x:x*2] + "x" + scid[x*2:]
	block, _ := strconv.Atoi(scid[:x])
	tx, _ := strconv.Atoi(scid[x : x*2])
	output, _ := strconv.Atoi(scid[x*2:])

	return uint64(block<<40 | tx<<16 | output)
}

func parseShortChannelId(scid string) (id string, ok bool) {
	spl := strings.Split(scid, "x")
	if len(spl) != 3 {
		return "", false
	}

	scid = spl[0] + spl[1] + spl[2]

	for i := 0; i < len(scid)/2; i += 2 {
		b, err := strconv.Atoi(scid[i : i+2])
		if err != nil {
			return "", false
		}
		letter := string([]byte{uint8(b + 40)})
		id += letter
	}

	return
}

func makeKeys(id string) (*btcec.PrivateKey, *btcec.PublicKey) {
	v := sha256.Sum256([]byte(s.SecretKey + ":" + id))
	return btcec.PrivKeyFromBytes(btcec.S256(), v[:])
}

func makePreimage(id string) []byte {
	v := sha256.Sum256([]byte(s.SecretKey + ":" + id))
	return v[:]
}
