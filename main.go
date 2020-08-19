package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	lightning "github.com/fiatjaf/lightningd-gjson-rpc"
	"github.com/fiatjaf/lightningd-gjson-rpc/plugin"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"gopkg.in/redis.v5"
)

type Settings struct {
	ServiceId   string `envconfig:"SERVICE_ID" default:"etleneum.com"`
	ServiceURL  string `envconfig:"SERVICE_URL" required:"true"`
	Port        string `envconfig:"PORT" required:"true"`
	SecretKey   string `envconfig:"SECRET_KEY" default:"etleneum"`
	PostgresURL string `envconfig:"DATABASE_URL" required:"true"`
	RedisURL    string `envconfig:"REDIS_URL" required:"true"`

	GitHubRepoOwner string `envconfig:"GITHUB_REPO_OWNER"`
	GitHubRepoName  string `envconfig:"GITHUB_REPO_NAME"`
	GitHubToken     string `envconfig:"GITHUB_TOKEN"`

	InitialContractCostSatoshis int64 `envconfig:"INITIAL_CONTRACT_COST_SATOSHIS" default:"970"`
	FixedCallCostSatoshis       int64 `envconfig:"FIXED_CALL_COST_SATOSHIS" default:"1"`

	NodeId   string
	FreeMode bool
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
	http.DefaultClient = &http.Client{Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxConnsPerHost:     10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     10 * time.Second,
		DisableCompression:  true,
	}}

	if isRunningAsPlugin() {
		p := plugin.Plugin{
			Name:    "etleneum",
			Version: "v2.0",
			Dynamic: true,
			Hooks: []plugin.Hook{
				{
					"htlc_accepted",
					htlc_accepted,
				},
			},
			OnInit: func(p *plugin.Plugin) {
				// set environment from envfile (hack)
				envpath := "etleneum.env"
				if !filepath.IsAbs(envpath) {
					// expand tlspath from lightning dir
					envpath = filepath.Join(filepath.Dir(p.Client.Path), envpath)
				}

				if _, err := os.Stat(envpath); err != nil {
					log.Fatal().Err(err).Str("path", envpath).Msg("envfile not found")
				}

				godotenv.Load(envpath)

				// globalize the lightning rpc client
				ln = p.Client

				// get our own nodeid
				res, err := ln.Call("getinfo")
				if err != nil {
					log.Fatal().Err(err).Msg("couldn't call getinfo")
				}
				s.NodeId = res.Get("id").String()

				// start the server
				server()
			},
		}

		p.Run()
	} else {
		// when not running as a plugin this will operate on the free mode
		s.FreeMode = true

		// start the server
		server()
	}
}

func server() {
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
	router.Path("/~/contract/{ctid}/state").Methods("POST").HandlerFunc(getContractState)
	router.Path("/~/contract/{ctid}/state/{jq}").Methods("GET").HandlerFunc(getContractState)
	router.Path("/~/contract/{ctid}/funds").Methods("GET").HandlerFunc(getContractFunds)
	router.Path("/~/contract/{ctid}").Methods("DELETE").HandlerFunc(deleteContract)
	router.Path("/~/contract/{ctid}/calls").Methods("GET").HandlerFunc(listCalls)
	router.Path("/~/contract/{ctid}/call").Methods("POST").HandlerFunc(prepareCall)
	router.Path("/~~~/contract/{ctid}").Methods("GET").HandlerFunc(contractStream)
	router.Path("/~/call/{callid}").Methods("GET").HandlerFunc(getCall)
	router.Path("/~/call/{callid}").Methods("PATCH").HandlerFunc(patchCall)
	router.Path("/lnurl/contract/{ctid}/call/{method}/{msatoshi}").
		Methods("GET").HandlerFunc(lnurlPayParams)
	router.Path("/lnurl/contract/{ctid}/call/{method}").
		Methods("GET").HandlerFunc(lnurlPayParams)
	router.Path("/lnurl/call/{callid}").Methods("GET").HandlerFunc(lnurlPayValues)
	router.Path("/~~~/session").Methods("GET").HandlerFunc(lnurlSession)
	router.Path("/lnurl/auth").Methods("GET").HandlerFunc(lnurlAuth)
	router.Path("/~/session/refresh").Methods("GET").HandlerFunc(refreshBalance)
	router.Path("/lnurl/withdraw").Methods("GET").HandlerFunc(lnurlWithdraw)
	router.Path("/lnurl/withdraw/callback").Methods("GET").HandlerFunc(lnurlWithdrawCallback)
	router.Path("/~/session/logout").Methods("POST").HandlerFunc(logout)
	router.Path("/^/webhook/github").Methods("POST").HandlerFunc(handleGitHubWebhook)
	router.Path("/_/decode-scid/{scid}").Methods("GET").HandlerFunc(handleDecodeScid)
	router.Path("/_/call-details/{callid}").Methods("GET").HandlerFunc(handleCallDetails)
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
		log.Error().Err(err).Str("file", "static/index.html").
			Msg("make sure you generated bindata.go without -debug")
		return
	}
	fstat, _ := indexf.Stat()
	http.ServeContent(w, r, "static/index.html", fstat.ModTime(), indexf)
	return
}

func isRunningAsPlugin() bool {
	pid := os.Getppid()
	res, _ := exec.Command(
		"ps", "-p", strconv.Itoa(pid), "-o", "comm=",
	).CombinedOutput()

	return strings.TrimSpace(string(res)) == "lightningd"
}
