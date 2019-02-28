package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/fiatjaf/etleneum/runlua"
	"github.com/fiatjaf/etleneum/runlua/assets"
	"github.com/fiatjaf/etleneum/types"
	"github.com/fiatjaf/ln-decodepay"
	sqlxtypes "github.com/jmoiron/sqlx/types"
	"github.com/tidwall/gjson"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()
	app.ErrWriter = os.Stderr
	app.Writer = os.Stdout
	app.Name = "runcall"
	app.Usage = "Run a call on an Etleneum contract."
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "contract",
			Usage: "File with the full lua code for the contract.",
		},
		cli.StringFlag{
			Name:  "state",
			Value: "{}",
			Usage: "Current contract state as JSON string.",
		},
		cli.StringFlag{
			Name:  "method",
			Value: "__init__",
			Usage: "Contract method to run.",
		},
		cli.StringFlag{
			Name:  "payload",
			Value: "{}",
			Usage: "Payload to send with the call as a JSON string.",
		},
		cli.IntFlag{
			Name:  "satoshis",
			Value: 0,
			Usage: "Satoshis to include in the call.",
		},
	}
	app.Action = func(c *cli.Context) error {
		bsandboxCode, _ := assets.Asset("runlua/assets/sandbox.lua")
		sandboxCode := string(bsandboxCode)

		// contract code
		contractFile := c.String("contract")
		if contractFile == "" {
			fmt.Fprint(app.ErrWriter, "missing contract file.")
			os.Exit(1)
		}
		bcontractCode, err := ioutil.ReadFile(contractFile)
		if err != nil {
			fmt.Fprintf(app.ErrWriter, "failed to read contract file '%s'.", contractFile)
			os.Exit(1)
		}

		// payload

		state, paid, payments, returned, err := runlua.RunCall(
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
			types.Contract{
				Code:  string(bcontractCode),
				State: sqlxtypes.JSONText([]byte(c.String("state"))),
			},
			types.Call{
				Satoshis: c.Int("satoshis"),
				Method:   c.String("method"),
				Payload:  sqlxtypes.JSONText([]byte(c.String("payload"))),
			},
		)
		if err != nil {
			fmt.Fprintln(app.ErrWriter, "execution error: "+err.Error())
			os.Exit(3)
		}

		json.NewEncoder(app.Writer).Encode(struct {
			Paid          int
			Payments      []string
			ReturnedValue interface{}
			State         interface{}
		}{paid, payments, returned, state})

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprint(app.ErrWriter, err.Error())
		os.Exit(2)
	}
}
