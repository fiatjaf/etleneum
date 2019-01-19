etleneum: $(shell find . -name "*.go")
	go build -o ./etleneum

watch:
	find . -name "*.go" | entr -r bash -c 'make etleneum && ./etleneum'
