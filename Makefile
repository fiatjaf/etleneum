all: etleneum runcall

etleneum: $(shell find . -name "*.go") static/bundle.js
	go build

runcall: runlua/runlua.go runlua/cmd/runcall/main.go
	cd runlua/cmd/runcall && CC=$$(which musl-gcc) go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o ../../../runcall

static/bundle.js: $(shell find client)
	GITHUB_REPO=etleneum/database-dev ./node_modules/.bin/rollup -c

deploy_test: etleneum
	GITHUB_REPO=etleneum/database-dev ./node_modules/.bin/rollup -c
	CC=$$(which musl-gcc) go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o ./etleneum
	ssh root@hulsmann 'systemctl stop etleneum-test'
	scp etleneum hulsmann:etleneum-test/etleneum
	ssh root@hulsmann 'systemctl start etleneum-test'

deploy: etleneum
	PRODUCTION=true GITHUB_REPO=etleneum/database ./node_modules/.bin/rollup -c
	CC=$$(which musl-gcc) go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o ./etleneum
	rsync etleneum hulsmann:.lightning2/plugins/etleneum-new
	ssh hulsmann 'ln2 plugin stop etleneum; mv .lightning2/plugins/etleneum-new .lightning2/plugins/etleneum; ln2 plugin start $$HOME/.lightning2/plugins/etleneum'
