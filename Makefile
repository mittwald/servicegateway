GOFILES=$(wildcard **/*.go)

servicegateway: ${GOFILES}
	go build -tags netgo

build-static:
	$(MAKE) -C static

docker: servicegateway build-static Dockerfile
	docker build -t mittwald/servicegateway .
