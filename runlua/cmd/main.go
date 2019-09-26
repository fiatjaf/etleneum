package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/fiatjaf/etleneum/runlua"
	"github.com/fiatjaf/etleneum/runlua/assets"
	"github.com/fiatjaf/etleneum/types"
	sqlxtypes "github.com/jmoiron/sqlx/types"
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
		cli.IntFlag{
			Name:  "funds",
			Usage: "Contract will have this amount of funds (in satoshi).",
		},
		cli.StringFlag{
			Name:  "caller",
			Usage: "Id of the account that is making the call.",
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
		cli.StringSliceFlag{
			Name:  "http",
			Usage: "HTTP response to mock. Can be called multiple times. Will return the multiple values in order to each HTTP call made by the contract.",
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

		// http mock
		httpResponses := c.StringSlice("http")
		httpRespIndex := 0
		returnHttp := func(r *http.Request) (*http.Response, error) {
			if httpRespIndex < len(httpResponses) {
				// use a mock
				respText := httpResponses[httpRespIndex]
				body := bytes.NewBufferString(respText)
				w := &http.Response{
					Status:        "200 OK",
					StatusCode:    200,
					Proto:         "HTTP/1.0",
					ProtoMajor:    1,
					ProtoMinor:    0,
					Request:       r,
					Body:          ioutil.NopCloser(body),
					ContentLength: int64(body.Len()),
				}
				httpRespIndex++
				return w, nil
			}
			return http.DefaultClient.Do(r)
		}

		contractFunds := c.Int("funds") * 1000

		state, err := runlua.RunCall(
			sandboxCode,
			returnHttp,
			func() (contractFunds int, err error) { return contractFunds, nil },
			func(target string, msat int) (msatoshiSent int, err error) {
				contractFunds -= msat
				return msat, nil
			},
			func() (userBalance int, err error) { return 99999, nil },
			func(target string, msat int) (msatoshiSent int, err error) { return msat, nil },
			types.Contract{
				Code:  string(bcontractCode),
				State: sqlxtypes.JSONText([]byte(c.String("state"))),
				Funds: contractFunds,
			},
			types.Call{
				Id:       "callid",
				Msatoshi: 1000 * c.Int("satoshis"),
				Method:   c.String("method"),
				Payload:  sqlxtypes.JSONText([]byte(c.String("payload"))),
				Caller:   c.String("caller"),
			},
		)
		if err != nil {
			fmt.Fprintln(app.ErrWriter, "execution error: "+err.Error())
			os.Exit(3)
		}

		json.NewEncoder(app.Writer).Encode(state)

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprint(app.ErrWriter, err.Error())
		os.Exit(2)
	}
}
