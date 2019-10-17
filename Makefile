all: etleneum runcall

etleneum: $(shell find . -name "*.go") bindata.go runlua/assets/bindata.go
	go build -o ./etleneum

runcall: runlua/runlua.go runlua/assets/bindata.go runlua/cmd/main.go
	cd runlua/cmd && go build -o ../../runcall

prod: $(shell find . -name "*.go") static/bundle.min.js static/style.min.css
	mv static/bundle.min.js static/bundle.js
	mv static/style.min.css static/style.css
	go-bindata -pkg assets -o runlua/assets/bindata.go runlua/assets/...
	go-bindata -o bindata.go static/...
	go build -o ./etleneum

watch:
	find . -name "*.go" | entr -r bash -c 'make etleneum && ./etleneum'

bindata.go: $(shell find static)
	go-bindata -debug -o bindata.go static/...

runlua/assets/bindata.go: $(shell find runlua/assets ! -name "bindata.go")
	go-bindata -pkg assets -o runlua/assets/bindata.go -ignore=.*\.go runlua/assets/...

static/bundle.js: $(shell find client)
	./node_modules/.bin/rollup -c
