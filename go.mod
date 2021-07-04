module github.com/fiatjaf/etleneum

go 1.14

require (
	github.com/aarzilli/golua v0.0.0-20190714183732-fc27908ace94
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da
	github.com/btcsuite/btcd v0.20.1-beta.0.20200515232429-9f0179fd2c46
	github.com/btcsuite/btcutil v1.0.2
	github.com/elazarl/go-bindata-assetfs v1.0.0
	github.com/fiatjaf/go-lnurl v1.0.0
	github.com/fiatjaf/hashbow v1.0.0
	github.com/fiatjaf/lightningd-gjson-rpc v1.0.0
	github.com/fiatjaf/ln-decodepay v1.0.0
	github.com/fiatjaf/lunatico v1.0.0
	github.com/fogleman/gg v1.3.0
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0
	github.com/google/go-github/v36 v36.0.0
	github.com/gorilla/mux v1.7.4
	github.com/itchyny/gojq v0.10.3
	github.com/jmoiron/sqlx v1.2.0
	github.com/joho/godotenv v1.3.0
	github.com/k0kubun/colorstring v0.0.0-20150214042306-9440f1994b88 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/lib/pq v1.7.0
	github.com/lightningnetwork/lightning-onion v1.0.1
	github.com/lightningnetwork/lnd v0.10.1-beta
	github.com/lucsky/cuid v1.0.2
	github.com/orcaman/concurrent-map v0.0.0-20190826125027-8c72a8bb44f6
	github.com/rs/cors v1.7.0
	github.com/rs/zerolog v1.19.0
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/tidwall/gjson v1.6.0
	github.com/yudai/gojsondiff v1.0.0
	github.com/yudai/golcs v0.0.0-20170316035057-ecda9a501e82 // indirect
	github.com/yudai/pp v2.0.1+incompatible // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/image v0.0.0-20200618115811-c13761719519 // indirect
	gopkg.in/antage/eventsource.v1 v1.0.0-20150318155416-803f4c5af225
	gopkg.in/redis.v5 v5.2.9
	gopkg.in/urfave/cli.v1 v1.20.0
)

replace github.com/fiatjaf/go-lnurl => /home/fiatjaf/comp/go-lnurl
