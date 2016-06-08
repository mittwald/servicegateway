GOFILES=$(wildcard **/*.go)
GOVERSION=1.6

servicegateway: ${GOFILES}
	docker run --rm -v $(PWD):/go/src/github.com/mittwald/servicegateway -w /go/src/github.com/mittwald/servicegateway golang:$(GOVERSION) go build -tags netgo

build-static:
	$(MAKE) -C static

docker: servicegateway build-static Dockerfile
	docker build -t mittwald/servicegateway .
