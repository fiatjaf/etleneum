package runlua

import (
	"encoding/json"

	"github.com/fiatjaf/etleneum/runlua"
	"github.com/fiatjaf/etleneum/runlua/assets"
	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/ln-decodepay"
	"github.com/tidwall/gjson"
)

func main() {
	bsandboxCode, _ := assets.Asset("static/sandbox.lua")
	sandboxCode := string(bsandboxCode)

	runlua.RunCall(
		sandboxCode,
		func(bolt11 string) (gjson.Result, error) {
			d, err := decodepay.Decodepay(bolt11)
			if err != nil {
				return gjson.Result{}, err
			}

			jsonb, err := json.Marshal(d)
			if err != nil {
				return gjson.Result{}, err
			}

			return gjson.ParseBytes(jsonb), nil
		},
		types.Contract{},
		types.Call{},
	)
}
