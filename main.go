package main

import (
	"net/http"
	"os"
	"time"

	"github.com/fiatjaf/lightningd-gjson-rpc"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

type Settings struct {
	ServiceId   string `envconfig:"SERVICE_ID" default:"lntxbot"`
	ServiceURL  string `envconfig:"SERVICE_URL" required:"true"`
	Port        string `envconfig:"PORT" required:"true"`
	PostgresURL string `envconfig:"DATABASE_URL" required:"true"`
	SocketPath  string `envconfig:"SOCKET_PATH" required:"true"`
}

var err error
var s Settings
var pg *sqlx.DB
var ln *lightning.Client
var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})

func main() {
	err = envconfig.Process("", &s)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't process envconfig.")
	}

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log = log.With().Timestamp().Logger()

	// postgres connection
	pg, err = sqlx.Connect("postgres", s.PostgresURL)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't connect to postgres")
	}

	// lightningd connection
	ln, err = lightning.Connect(s.SocketPath)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't connect to lightning-rpc")
	}

	// pause here until lightningd works
	probeLightningd()

	res, err := runLua(1, "buytoken", "etleneum.389d4wbei7", map[string]interface{}{"amount": 2.0, "user": "fiatjaf"})
	log.Print(res, err)
	os.Exit(0)

	// http server
	r := mux.NewRouter()

	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:" + s.Port,
		WriteTimeout: 25 * time.Second,
		ReadTimeout:  25 * time.Second,
	}
	log.Info().Str("port", s.Port).Msg("listening.")
	srv.ListenAndServe()
}

func probeLightningd() {
	nodeinfo, err := ln.Call("getinfo")
	if err != nil {
		log.Warn().Err(err).Msg("can't talk to lightningd. retrying.")
		time.Sleep(time.Second * 5)
		probeLightningd()
		return
	}
	log.Info().
		Str("id", nodeinfo.Get("id").String()).
		Str("alias", nodeinfo.Get("alias").String()).
		Int64("channels", nodeinfo.Get("num_active_channels").Int()).
		Int64("blockheight", nodeinfo.Get("blockheight").Int()).
		Str("version", nodeinfo.Get("version").String()).
		Msg("lightning node connected")
}
