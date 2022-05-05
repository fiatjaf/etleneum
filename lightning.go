package main

import (
	"bytes"
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

var BOGUS_SECRET = [32]byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}

func makeInvoice(
	isFreeMode bool, // should be true only on dev environments and test.etleneum.com
	ctid string, // pubkey is based on this
	id string, // call or contract id: scid and preimage based on this
	desc string,
	deschash *[32]byte,
	main_price int64, // in msatoshi
	cost int64, // will be added as routing fees in the last channel
) (bolt11 string, err error) {
	sk, _ := makeKeys(ctid)
	preimage := makePreimage(id)
	channelid := makeShortChannelId(id)

	var network *chaincfg.Params
	var ournodeid *btcec.PublicKey
	if isFreeMode {
		network = &chaincfg.RegressionNetParams
		sk, _ := btcec.NewPrivateKey(btcec.S256())
		ournodeid = sk.PubKey()
	} else {
		network = &chaincfg.MainNetParams
		nodeid, _ := hex.DecodeString(s.NodeId)
		ournodeid, err = btcec.ParsePubKey(nodeid, btcec.S256())
		if err != nil {
			return "", fmt.Errorf("error parsing our own nodeid: %w", err)
		}

	}

	var addDescription func(*zpay32.Invoice)
	if deschash != nil {
		addDescription = zpay32.DescriptionHash(*deschash)
	} else {
		addDescription = zpay32.Description(desc)
	}

	invoice, err := zpay32.NewInvoice(
		network,
		sha256.Sum256(preimage),
		time.Now(),
		zpay32.RouteHint([]zpay32.HopHint{
			{
				NodeID:                    ournodeid,
				ChannelID:                 channelid,
				FeeBaseMSat:               uint32(cost),
				FeeProportionalMillionths: 0,
				CLTVExpiryDelta:           144,
			},
		}),
		zpay32.Amount(lnwire.MilliSatoshi(main_price)),
		zpay32.Expiry(time.Hour*24),
		zpay32.Features(&lnwire.FeatureVector{
			RawFeatureVector: lnwire.NewRawFeatureVector(
				lnwire.PaymentAddrOptional,
				lnwire.TLVOnionPayloadOptional,
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

var SHORT_CHANNEL_ID_CHARACTERS = []uint8{'_', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}

// makeShortChannelId turns a call or contract id into a short_channel_id (64 bits)
func makeShortChannelId(id string) (scid uint64) {
	// we use 61 of the 64 bits available for this
	// the first 3 bits [63, 62, 61] are blank

	// the bit 60 is used to identify if this is a call (r) or contract (c).
	var typebit uint64
	if id[0] == 'c' {
		typebit = 1
	} else {
		typebit = 0
	}
	scid = scid | typebit<<60

	// then we fit the rest of letters and digits into a 6-bit custom encoding,
	id = id[1:]

	// so there are room for 10 characters, which is what we need to fit a cuid slug.
	// since the cuid slug can be between 7 and 10, we also accomodate for blank
	// strings at the end by having an empty character ('_') encoded in 6 bits too.
	// BUG: because of the monotonically increasing counter used in cuids, they don't
	// have a limit of characters, it can increase indefinitely, so after some hundred
	// calls it will get to 11 characters and this will break. a restart fixes it.
	arreda := 60
	for _, letter := range []byte(id) {
		n := bytes.Index(SHORT_CHANNEL_ID_CHARACTERS, []uint8{letter})
		arreda -= 6
		scid = scid | uint64(n)<<arreda
	}

	return
}

// parseShortChannelId is the reverse of makeShortChannelId
func parseShortChannelId(scid uint64) (id string, ok bool) {
	typebit := (scid >> 60) & 1
	if typebit == 0 {
		id += "r"
	} else {
		id += "c"
	}

	for arreda := 60 - 6; arreda >= 0; arreda -= 6 {
		n := int((scid >> arreda) & 63)

		if n > len(SHORT_CHANNEL_ID_CHARACTERS)-1 {
			return "", false
		}

		letter := SHORT_CHANNEL_ID_CHARACTERS[n]

		if letter == '_' {
			continue
		}

		id += string([]uint8{letter})
	}

	return id, true
}

func encodeShortChannelId(scid uint64) string {
	block := strconv.FormatUint((scid>>40)&0xFFFFFF, 10)
	tx := strconv.FormatUint((scid>>16)&0xFFFFFF, 10)
	out := strconv.FormatUint(scid&0xFFFF, 10)

	return block + "x" + tx + "x" + out
}

func decodeShortChannelId(scid string) (uint64, error) {
	spl := strings.Split(scid, "x")

	x, err := strconv.ParseUint(spl[0], 10, 64)
	if err != nil {
		return 0, err
	}
	y, err := strconv.ParseUint(spl[1], 10, 64)
	if err != nil {
		return 0, err
	}
	z, err := strconv.ParseUint(spl[2], 10, 64)
	if err != nil {
		return 0, err
	}

	return ((x & 0xFFFFFF) << 40) | ((y & 0xFFFFFF) << 16) | (z & 0xFFFF), nil
}

func makeKeys(id string) (*btcec.PrivateKey, *btcec.PublicKey) {
	v := sha256.Sum256([]byte(s.SecretKey + ":" + id))
	return btcec.PrivKeyFromBytes(btcec.S256(), v[:])
}

func makePreimage(id string) []byte {
	v := sha256.Sum256([]byte(s.SecretKey + ":" + id))
	return v[:]
}
