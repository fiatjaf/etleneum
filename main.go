package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/fiatjaf/lightningd-gjson-rpc"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"gopkg.in/redis.v5"
)

type Settings struct {
	ServiceId   string `envconfig:"SERVICE_ID" default:"etleneum"`
	ServiceURL  string `envconfig:"SERVICE_URL" required:"true"`
	Port        string `envconfig:"PORT" required:"true"`
	HashidSalt  string `envconfig:"HASHID_SALT" default:"etleneum"`
	PostgresURL string `envconfig:"DATABASE_URL" required:"true"`
	RedisURL    string `envconfig:"REDIS_URL" required:"true"`
	SocketPath  string `envconfig:"SOCKET_PATH" required:"true"`

	InitialContractCostSatoshis int `envconfig:"INITIAL_CONTRACT_COST_SATOSHIS" default:"100"`
	FixedCallCostSatoshis       int `envconfig:"FIXED_CALL_COST_SATOSHIS" default:"1"`
	PaymentRetrySeconds         int `envconfig:"PAYMENT_RETRY_SECONDS" default:"30"`

	Development bool `envconfig:"DEV" default:"false"`
}

var err error
var s Settings
var pg *sqlx.DB
var ln *lightning.Client
var rds *redis.Client
var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})
var httpPublic = &assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: ""}

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

	// redis connection
	rurl, _ := url.Parse(s.RedisURL)
	pw, _ := rurl.User.Password()
	rds = redis.NewClient(&redis.Options{
		Addr:     rurl.Host,
		Password: pw,
	})
	if err := rds.Ping().Err(); err != nil {
		log.Fatal().Err(err).Str("url", s.RedisURL).
			Msg("failed to connect to redis")
	}

	// lightningd connection
	lastinvoiceindex, _ := rds.Get("lastinvoiceindex").Int64()
	ln = &lightning.Client{
		Path:             s.SocketPath,
		LastInvoiceIndex: int(lastinvoiceindex),
		PaymentHandler:   handleInvoicePaid,
	}
	ln.ListenForInvoices()

	// pause here until lightningd works
	probeLightningd()

	// http server
	router := mux.NewRouter()
	router.PathPrefix("/static/").Methods("GET").Handler(http.FileServer(httpPublic))
	router.Path("/favicon.ico").Methods("GET").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			iconf, _ := httpPublic.Open("static/icon.png")
			fstat, _ := iconf.Stat()
			http.ServeContent(w, r, "static/icon.png", fstat.ModTime(), iconf)
			return
		})
	router.Path("/~/contracts").Methods("GET").HandlerFunc(listContracts)
	router.Path("/~/contract").Methods("POST").HandlerFunc(prepareContract)
	router.Path("/~/contract/{ctid}/refill/{sats}").Methods("GET").HandlerFunc(refillContract)
	router.Path("/~/contract/{ctid}").Methods("GET").HandlerFunc(getContract)
	router.Path("/~/contract/{ctid}/state").Methods("GET").HandlerFunc(getContractState)
	router.Path("/~/contract/{ctid}/funds").Methods("GET").HandlerFunc(getContractFunds)
	router.Path("/~/contract/{ctid}").Methods("POST").HandlerFunc(makeContract)
	router.Path("/~/contract/{ctid}/calls").Methods("GET").HandlerFunc(listCalls)
	router.Path("/~/contract/{ctid}/call").Methods("POST").HandlerFunc(prepareCall)
	router.Path("/~/call/{callid}").Methods("GET").HandlerFunc(getCall)
	router.Path("/~/call/{callid}").Methods("PATCH").HandlerFunc(patchCall)
	router.Path("/~/call/{callid}").Methods("POST").HandlerFunc(makeCall)
	router.Path("/~/retry/{bolt11}").Methods("POST").HandlerFunc(retryPayment)
	router.PathPrefix("/").Methods("GET").HandlerFunc(serveClient)

	srv := &http.Server{
		Handler: cors.New(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "HEAD", "POST", "PATCH", "DELETE", "PUT"},
			AllowCredentials: false,
		}).Handler(router),
		Addr:         "0.0.0.0:" + s.Port,
		WriteTimeout: 25 * time.Second,
		ReadTimeout:  25 * time.Second,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, syscall.SIGTERM, syscall.SIGINT)
		<-sigint

		log.Debug().Msg("Received an interrupt signal, shutting down.")
		if err := srv.Shutdown(context.Background()); err != nil {
			// error from closing listeners, or context timeout:
			log.Warn().Err(err).Msg("HTTP server shutdown")
		}

		close(idleConnsClosed)
	}()

	log.Info().Str("port", s.Port).Msg("listening.")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Warn().Err(err).Msg("listenAndServe")
	}

	<-idleConnsClosed
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

func serveClient(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	indexf, err := httpPublic.Open("static/index.html")
	if err != nil {
		log.Error().Err(err).Str("file", "static/index.html").Msg("make sure you generated bindata.go without -debug")
		return
	}
	fstat, _ := indexf.Stat()
	http.ServeContent(w, r, "static/index.html", fstat.ModTime(), indexf)
	return
}
