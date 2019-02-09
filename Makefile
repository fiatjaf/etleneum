all: etleneum

etleneum: $(shell find . -name "*.go")
	go build -o ./etleneum

prod: $(shell find . -name "*.go") static/bundle.min.js static/style.min.css
	mv static/bundle.min.js static/bundle.js
	mv static/style.min.css static/style.css
	go-bindata -o bindata.go static/...
	go build -o ./etleneum

watch:
	find . -name "*.go" | entr -r bash -c 'make etleneum && ./etleneum'

bindata.go: $(shell find static)
	go-bindata -debug -o bindata.go static/...

static/bundle.js: $(shell find client -name "*.re" -o -name "*.js" ! -name "*.bs.js")
	bsb -make-world
	./node_modules/.bin/browserify client/App.bs.js -dv --outfile static/bundle.js

static/bundle.min.js: $(shell find client -name "*.re" -o -name "*.js" ! -name "*.bs.js")
	bsb -make-world
	./node_modules/.bin/browserify client/App.bs.js -g [ envify --NODE_ENV production ] -g uglifyify | ./node_modules/.bin/terser --compress --mangle > static/bundle.min.js

static/style.css: client/style.styl
	./node_modules/.bin/stylus < client/style.styl > static/style.css

static/style.min.css: client/style.styl
	./node_modules/.bin/stylus -c < client/style.styl > static/style.min.css
