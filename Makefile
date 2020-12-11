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
	ssh root@hulsmann 'systemctl stop etleneum-test'
	scp etleneum hulsmann:etleneum-test/etleneum
	ssh root@hulsmann 'systemctl start etleneum-test'

deploy: etleneum
	rsync etleneum hulsmann:.lightning2/plugins/etleneum-new
	ssh hulsmann 'ln2 plugin stop etleneum; mv .lightning2/plugins/etleneum-new .lightning2/plugins/etleneum; ln2 plugin start $$HOME/.lightning2/plugins/etleneum'
