package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/aead/chacha20"
	"github.com/btcsuite/btcd/btcec"
	"github.com/fiatjaf/etleneum/data"
	"github.com/fiatjaf/lightningd-gjson-rpc/plugin"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/lnwire"
)

var (
	continueHTLC = map[string]interface{}{"result": "continue"}
	failHTLC     = map[string]interface{}{"result": "fail", "failure_code": 16392}
)

func htlc_accepted(p *plugin.Plugin, params plugin.Params) (resp interface{}) {
	msatoshi := params.Get("htlc.amount_msat").Int()
	scid := params.Get("onion.short_channel_id").String()
	if scid == "0x0x0" {
		// payment coming to this node, accept it
		return continueHTLC
	}

	hash := params.Get("htlc.payment_hash").String()

	for rds == nil || !data.Initialized {
		p.Log("htlc_accepted: waiting until redis and filesystem are available.")
		time.Sleep(1 * time.Second)
	}

	bscid, err := decodeShortChannelId(scid)
	if err != nil {
		p.Logf("onion.short_channel_id is not in the usual format - continue")
		return continueHTLC
	}

	id, ok := parseShortChannelId(bscid)
	if !ok {
		// it's not an invoice for an etleneum call or contract
		return continueHTLC
	}

	if id[0] != 'c' && id[0] != 'r' {
		// it's not an invoice for an etleneum call or contract
		p.Logf("parsed id is not an etleneum payment (%s) - continue", id)
		return continueHTLC
	}

	// ensure that we can derive the correct preimage for this payment
	preimage := makePreimage(id)
	preimageHex := hex.EncodeToString(preimage)
	derivedHash := sha256.Sum256(preimage)
	derivedHashHex := hex.EncodeToString(derivedHash[:])
	if hash != derivedHashHex {
		p.Logf("we have a preimage %s, but its hash %s didn't match the expected hash %s - fail with incorrect_or_unknown_payment_details", preimageHex, derivedHashHex, hash)

		// get keys stuff so we can return a wrapped onion to pre-pay probes
		nextOnion, err := hex.DecodeString(params.Get("onion.next_onion").String())
		if err != nil {
			p.Logf("lightningd has sent us an invalid onion.next_onion: %s",
				err.Error())
			return failHTLC
		}

		var nextOnionPacket sphinx.OnionPacket
		err = nextOnionPacket.Decode(bytes.NewBuffer(nextOnion))
		if err != nil {
			p.Logf("couldn't parse onion.next_onion: %s", err.Error())
			return failHTLC
		}

		// private key for the last hop
		var ctid string
		if id[0] == 'c' {
			ctid = id
		} else if id[0] == 'r' {
			call, err := callFromRedis(id)
			if err != nil {
				return continueHTLC
			}
			ctid = call.ContractId
		}
		lastHopKey, _ := makeKeys(ctid)

		// bolt04 shared key stuff: ecdh() then sha256()
		s := &btcec.PublicKey{}
		s.X, s.Y = btcec.S256().ScalarMult(
			nextOnionPacket.EphemeralKey.X,
			nextOnionPacket.EphemeralKey.Y,
			lastHopKey.D.Bytes(),
		)
		lastHopSharedSecret := sha256.Sum256(s.SerializeCompressed())

		// produce the error as if we were the last hop
		failure := lnwire.NewFailIncorrectDetails(lnwire.MilliSatoshi(msatoshi), 0)
		var payload bytes.Buffer
		if err := lnwire.EncodeFailure(&payload, failure, 0); err != nil {
			panic(err)
		}
		data := payload.Bytes()

		// hmac the payload
		umKey := generateKey("um", lastHopSharedSecret[:])
		mac := hmac.New(sha256.New, umKey[:])
		mac.Write(data)
		h := mac.Sum(nil)
		failureOnion := append(h, data...)

		// obfuscate/wrap the message as if we were the last hop
		ammagKey := generateKey("ammag", lastHopSharedSecret[:])
		placeholder := make([]byte, len(failureOnion))
		xor(
			placeholder,
			failureOnion,
			generateCipherStream(ammagKey, uint(len(failureOnion))),
		)
		failureOnion = placeholder

		// return the onion as failure_onion and lightningd will wrap it
		return map[string]interface{}{
			"result":        "fail",
			"failure_onion": hex.EncodeToString(failureOnion),
		}
	}

	// run the call
	if id[0] == 'c' {
		ok = contractPaymentReceived(id, msatoshi)
	} else if id[0] == 'r' {
		ok = callPaymentReceived(id, msatoshi)
	}

	// after the call succeeds, we resolve the payment
	if ok {
		p.Logf("call went ok. we have a preimage: %s - resolve", preimageHex)
		return map[string]interface{}{
			"result":      "resolve",
			"payment_key": preimageHex,
		}
	} else {
		// in case of call execution failure we just fail the payment
		p.Logf("call failed - fail")
		return failHTLC
	}
}

func contractPaymentReceived(contractId string, msatoshi int64) (ok bool) {
	// start the contract
	logger := log.With().Str("ctid", contractId).Logger()

	ct, err := contractFromRedis(contractId)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch contract from redis to activate")
		dispatchContractEvent(contractId,
			ctevent{contractId, "", "", 0, err.Error(), "internal"}, "contract-error")
		return false
	}

	if getContractCost(*ct) > msatoshi {
		return false
	}

	data.Start()

	// create initial contract
	err = data.CreateContract(ct.Id, ct.Name, ct.Readme, ct.Code)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to save contract on database")
		data.Abort()
		dispatchContractEvent(contractId,
			ctevent{contractId, "", "", 0, err.Error(), "internal"}, "contract-error")
		return false
	}

	// instantiate call (the __init__ special kind)
	call := &data.Call{
		ContractId: ct.Id,
		Id:         ct.Id, // same
		Method:     "__init__",
		Payload:    []byte("{}"),
		Cost:       getContractCost(*ct),
	}

	err = runCallGlobal(call, false)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to run call")
		data.Abort()
		dispatchContractEvent(contractId,
			ctevent{contractId, "", call.Method, 0, err.Error(), "runtime"},
			"contract-error")
		return false
	}

	// commit
	data.Finish("contract " + ct.Id + " created.")

	dispatchContractEvent(contractId,
		ctevent{contractId, "", call.Method, 0, "", ""}, "contract-created")
	logger.Info().Msg("contract is live")

	// saved. delete from redis.
	rds.Del("contract:" + contractId)

	return true
}

func callPaymentReceived(callId string, msatoshi int64) (ok bool) {
	// run the call
	logger := log.With().Str("callid", callId).Logger()

	call, err := callFromRedis(callId)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to fetch call from redis")
		return false
	}
	logger = logger.With().Str("ct", call.ContractId).Logger()

	if call.Msatoshi+call.Cost > msatoshi {
		// TODO: this is the place where we should handle MPP payments
		logger.Warn().Int64("got", msatoshi).Int64("needed", call.Msatoshi+call.Cost).
			Msg("insufficient payment amount")
		return false
	}
	// if msatoshi is bigger than needed we take it as a donation

	data.Start()
	logger.Info().Interface("call", call).Msg("call being made")

	// a normal call
	err = runCallGlobal(call, false)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to run call")
		data.Abort()
		dispatchContractEvent(call.ContractId,
			ctevent{callId, call.ContractId, call.Method, call.Msatoshi, err.Error(), "runtime"}, "call-error")

		return false
	}

	// commit
	data.Finish(call.Method + " " + call.Id + " executed on contract " + call.ContractId + ".")

	dispatchContractEvent(call.ContractId,
		ctevent{callId, call.ContractId, call.Method, call.Msatoshi, "", ""}, "call-made")

	// saved. delete from redis.
	rds.Del("call:" + call.Id)

	return true
}

func generateCipherStream(key [32]byte, numBytes uint) []byte {
	var nonce [8]byte
	cipher, err := chacha20.NewCipher(nonce[:], key[:])
	if err != nil {
		panic(err)
	}
	output := make([]byte, numBytes)
	cipher.XORKeyStream(output, output)

	return output
}

func xor(dst, a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		dst[i] = a[i] ^ b[i]
	}
	return n
}

func generateKey(keyType string, sharedKey []byte) [32]byte {
	mac := hmac.New(sha256.New, []byte(keyType))
	mac.Write(sharedKey)
	h := mac.Sum(nil)

	var key [32]byte
	copy(key[:], h[:32])

	return key
}
