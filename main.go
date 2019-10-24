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
	"github.com/orcaman/concurrent-map"
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

	InitialContractCostSatoshis int `envconfig:"INITIAL_CONTRACT_COST_SATOSHIS" default:"970"`
	FixedCallCostSatoshis       int `envconfig:"FIXED_CALL_COST_SATOSHIS" default:"1"`
	PaymentRetrySeconds         int `envconfig:"PAYMENT_RETRY_SECONDS" default:"30"`

	FreeMode bool `envconfig:"FREE_MODE" default:"false"`
}

var err error
var s Settings
var pg *sqlx.DB
var ln *lightning.Client
var rds *redis.Client
var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})
var httpPublic = &assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: ""}
var userstreams = cmap.New()
var contractstreams = cmap.New()

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
	if !s.FreeMode {
		lastinvoiceindex, _ := rds.Get("lastinvoiceindex").Int64()
		ln = &lightning.Client{
			Path:             s.SocketPath,
			LastInvoiceIndex: int(lastinvoiceindex),
			PaymentHandler:   handleInvoicePaid,
		}
		log.Debug().Int64("index", lastinvoiceindex).Msg("listening for invoices")
		ln.ListenForInvoices()
	}

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
	router.Path("/~/contract/{ctid}").Methods("GET").HandlerFunc(getContract)
	router.Path("/~/contract/{ctid}/state").Methods("GET").HandlerFunc(getContractState)
	router.Path("/~/contract/{ctid}/funds").Methods("GET").HandlerFunc(getContractFunds)
	router.Path("/~/contract/{ctid}").Methods("DELETE").HandlerFunc(deleteContract)
	router.Path("/~/contract/{ctid}/events").Methods("GET").HandlerFunc(listEvents)
	router.Path("/~/contract/{ctid}/calls").Methods("GET").HandlerFunc(listCalls)
	router.Path("/~/contract/{ctid}/call").Methods("POST").HandlerFunc(prepareCall)
	router.Path("/~/contract/{ctid}/stream").Methods("GET").HandlerFunc(contractStream)
	router.Path("/~/call/{callid}").Methods("GET").HandlerFunc(getCall)
	router.Path("/~/call/{callid}").Methods("PATCH").HandlerFunc(patchCall)
	router.Path("/~/refunds").Methods("GET").HandlerFunc(listRefunds)
	router.Path("/lnurl/refund/{preimage}").Methods("GET").HandlerFunc(lnurlRefund)
	router.Path("/lnurl/refund/callback").Methods("GET").HandlerFunc(lnurlRefund)
	router.Path("/lnurl/session").Methods("GET").HandlerFunc(lnurlSession)
	router.Path("/lnurl/auth").Methods("GET").HandlerFunc(lnurlAuth)
	router.Path("/lnurl/refresh").Methods("GET").HandlerFunc(refreshBalance)
	router.Path("/lnurl/withdraw").Methods("GET").HandlerFunc(lnurlWithdraw)
	router.Path("/lnurl/withdraw/callback").Methods("GET").HandlerFunc(lnurlWithdrawCallback)
	router.Path("/lnurl/logout").Methods("POST").HandlerFunc(logout)
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
