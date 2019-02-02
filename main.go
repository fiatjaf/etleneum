package main

import (
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/fiatjaf/lightningd-gjson-rpc"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
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
	ln, err = lightning.Connect(s.SocketPath)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't connect to lightning-rpc")
	}

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
	router.Path("/~/contract/{ctid}").Methods("GET").HandlerFunc(getContract)
	router.Path("/~/contract/{ctid}").Methods("POST").HandlerFunc(makeContract)
	router.Path("/~/contract/{ctid}/calls").Methods("GET").HandlerFunc(listCalls)
	router.Path("/~/contract/{ctid}/call").Methods("POST").HandlerFunc(prepareCall)
	router.Path("/~/call/{callid}").Methods("GET").HandlerFunc(getCall)
	router.Path("/~/call/{callid}").Methods("POST").HandlerFunc(makeCall)
	router.PathPrefix("/").Methods("GET").HandlerFunc(serveClient)

	srv := &http.Server{
		Handler:      router,
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

func serveClient(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	indexf, _ := httpPublic.Open("static/index.html")
	fstat, _ := indexf.Stat()
	http.ServeContent(w, r, "static/index.html", fstat.ModTime(), indexf)
	return
}
