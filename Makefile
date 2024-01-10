GO_VERSION=1.21.4
PKG_LIST := $(shell go list ./... | grep -v /vendor/)
GOLANGCI_VERSION := "0.0.33"

all: dep build-static

lint:
	docker run --rm -t \
		-v $(shell go env GOPATH):/go \
		-v ${CURDIR}:/app \
		-v $(HOME)/.cache:/home/mittwald-golangci/.cache \
		-w /app \
		-e GOFLAGS="-buildvcs=false" \
		quay.io/mittwald/golangci-lint:$(GOLANGCI_VERSION) \
			golangci-lint run -v --fix ./...

dep:
	go mod download
	go mod tidy


build-static:
	CGO_ENABLED=0 GOOS=linux go build -o servicegateway

servicegateway:
	docker run --rm -v $(PWD):/usr/src/github.com/mittwald/servicegateway -w /usr/src/github.com/mittwald/servicegateway golang:$(GO_VERSION) make

docker:
	make build-static
	docker build -t mittwald/servicegateway .

fmt:
	go fmt ${PKG_LIST}
