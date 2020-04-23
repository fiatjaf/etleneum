package runlua

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"golang.org/x/crypto/openpgp"
	openpgperrors "golang.org/x/crypto/openpgp/errors"
)

func lua_keybase_lookup(provider, name string) (username string, err error) {
	params := url.Values{}
	params.Set("fields", "basics")
	params.Set(provider, name)
	url := "https://keybase.io/_/api/1.0/user/lookup.json"
	resp, err := http.Get(url + "?" + params.Encode())
	if err != nil {
		log.Print(err)
		return "", err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Print(err)
		return "", err
	}

	gjson.GetBytes(b, "them").ForEach(func(_, match gjson.Result) bool {
		username = match.Get("basics.username").String()
		return false
	})

	log.Print(username)
	return username, nil
}

func lua_keybase_verify_signature(username, text, sig string) (ok bool, err error) {
	resp, err := http.Get("https://keybase.io/" + username + "/pgp_keys.asc")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("keybase returned status code %d", resp.StatusCode)
	}

	keyring, err := openpgp.ReadArmoredKeyRing(resp.Body)
	if err != nil {
		return false, err
	}

	sig, err = getSignatureBlockFromBundle(sig)
	if err != nil {
		return false, err
	}

	verification_target := strings.NewReader(text)
	signature := strings.NewReader(sig)

	_, err = openpgp.CheckArmoredDetachedSignature(keyring, verification_target, signature)
	if err != nil {
		if _, ok := err.(openpgperrors.SignatureError); ok {
			// this means the signature is wrong and not some kind of operational error
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func lua_keybase_verify_bundle(username, bundle string) (ok bool, err error) {
	sig, err := getSignatureBlockFromBundle(bundle)
	if err != nil {
		return false, err
	}
	text, err := getSignedMessageFromBundle(bundle)
	if err != nil {
		return false, err
	}

	return lua_keybase_verify_signature(username, text, sig)
}

func lua_keybase_extract_message(bundle string) (message string) {
	message, _ = getSignedMessageFromBundle(bundle)
	return
}

var signedMessageRe = regexp.MustCompile(`-----BEGIN PGP SIGNED MESSAGE-----\n(\w.*\n)*\n((.*\n?)*)\n-----BEGIN`)

func getSignedMessageFromBundle(bundle string) (message string, err error) {
	matches := signedMessageRe.FindStringSubmatch(bundle)
	if len(matches) != 4 {
		return "", errors.New("failed to find signed message in block")
	}
	return strings.TrimSpace(matches[2]), nil
}

func getSignatureBlockFromBundle(bundle string) (signatureBlock string, err error) {
	index := strings.Index(bundle, "-----BEGIN PGP SIGNATURE-----")
	if index == -1 {
		return "", errors.New("block doesn't contain a signature")
	}
	return bundle[index:], nil
}
