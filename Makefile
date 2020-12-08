GO_VERSION=1.15
PKG_LIST := $(shell go list ./... | grep -v /vendor/)

all: dep build-static

lint:
	golangci-lint run

dep:
	go get && go mod vendor -v

build-static:
	CGO_ENABLED=0 GOOS=linux go build -o servicegateway

servicegateway:
	docker run --rm -v $(PWD):/usr/src/github.com/mittwald/servicegateway -w /usr/src/github.com/mittwald/servicegateway golang:$(GO_VERSION) make

docker:
	make build-static
	docker build -t mittwald/servicegateway .

fmt:
	go fmt ${PKG_LIST}