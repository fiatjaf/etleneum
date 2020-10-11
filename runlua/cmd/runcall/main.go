package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"syscall"

	"github.com/fiatjaf/etleneum/runlua"
	"github.com/fiatjaf/etleneum/types"
	sqlxtypes "github.com/jmoiron/sqlx/types"
	"github.com/rs/zerolog"
	"gopkg.in/urfave/cli.v1"
)

var devNull = os.NewFile(uintptr(syscall.Stderr), "/dev/null")
var log = zerolog.New(devNull).Output(zerolog.ConsoleWriter{Out: devNull})

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
			Usage: "Current contract state as JSON string. Ignored when statefile is given.",
		},
		cli.StringFlag{
			Name:  "statefile",
			Usage: "File with the initial JSON state which will be overwritten.",
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
		cli.Float64Flag{
			Name:  "satoshis",
			Value: 0,
			Usage: "Satoshis to include in the call.",
		},
		cli.Int64Flag{
			Name:  "msatoshi",
			Value: 0,
			Usage: "Msatoshi to include in the call.",
		},
		cli.StringSliceFlag{
			Name:  "http",
			Usage: "HTTP response to mock. Can be called multiple times. Will return the multiple values in order to each HTTP call made by the contract.",
		},
	}
	app.Action = func(c *cli.Context) error {
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

		contractFunds := c.Int64("funds") * 1000

		var statejson []byte
		stateFile := c.String("statefile")
		if stateFile != "" {
			statejson, err = ioutil.ReadFile(stateFile)
			if err != nil {
				fmt.Fprintf(app.ErrWriter, "failed to read state file '%s'.", stateFile)
				os.Exit(1)
			}
		} else {
			statejson = []byte(c.String("state"))
		}

		msatoshi := c.Int64("msatoshi")
		if msatoshi == 0 {
			msatoshi = int64(1000 * c.Float64("satoshis"))
		}

		state, err := runlua.RunCall(
			log,
			os.Stderr,
			returnHttp,
			func(_ string) (interface{}, int64, error) {
				return nil, 0, errors.New("no external contracts in test environment")
			},
			func(_, _ string, _ interface{}, _ int64) error {
				return errors.New("no external contracts in test environment")
			},
			func() (contractFunds int, err error) { return contractFunds, nil },
			func(target string, msat int) (msatoshiSent int, err error) {
				contractFunds -= int64(msat)
				fmt.Fprintf(os.Stderr, "%dmsat sent to %s\n", msat, target)
				return msat, nil
			},
			func() (userBalance int, err error) { return 99999, nil },
			func(target string, msat int) (msatoshiSent int, err error) {
				fmt.Fprintf(os.Stderr, "%dmsat sent to %s\n", msat, target)
				return msat, nil
			},
			types.Contract{
				Code:  string(bcontractCode),
				State: sqlxtypes.JSONText(statejson),
				Funds: contractFunds,
			},
			types.Call{
				Id:       "callid",
				Msatoshi: msatoshi,
				Method:   c.String("method"),
				Payload:  sqlxtypes.JSONText([]byte(c.String("payload"))),
				Caller:   c.String("caller"),
			},
		)
		if err != nil {
			fmt.Fprintln(app.ErrWriter, "execution error: "+err.Error())
			os.Exit(3)
		}

		if stateFile != "" {
			f, err := os.Create(stateFile)
			if err != nil {
				fmt.Fprintf(app.ErrWriter,
					"failed to write state to file '%s'.", stateFile)
				os.Exit(4)
			}
			defer f.Close()
			json.NewEncoder(f).Encode(state)
		} else {
			json.NewEncoder(app.Writer).Encode(state)
		}

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprint(app.ErrWriter, err.Error())
		os.Exit(2)
	}
}
