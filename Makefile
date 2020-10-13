all: etleneum runcall

etleneum: $(shell find . -name "*.go") bindata.go
	CC=$$(which musl-gcc) go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o ./etleneum

runcall: runlua/runlua.go runlua/cmd/runcall/main.go
	cd runlua/cmd/runcall && CC=$$(which musl-gcc) go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o ../../../runcall

bindata.go: static/bundle.js static/index.html static/global.css static/bundle.css
	go-bindata -o bindata.go static/...

static/bundle.js: $(shell find client)
	./node_modules/.bin/rollup -c

deploy_test: etleneum
	ssh root@fuyue-421 'systemctl stop etleneum-test'
	scp etleneum fuyue-421:etleneum-test/etleneum
	ssh root@fuyue-421 'systemctl start etleneum-test'

deploy: etleneum
	scp etleneum aspidiske-402:.lightning/plugins/etleneum-new
	ssh aspidiske-402 'lightning/cli/lightning-cli plugin stop etleneum; mv .lightning/plugins/etleneum-new .lightning/plugins/etleneum; lightning/cli/lightning-cli plugin start $$HOME/.lightning/plugins/etleneum'
