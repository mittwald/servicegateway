GOFILES=$(wildcard **/*.go)

servicegateway: ${GOFILES}
	go build -tags netgo

docker: servicegateway Dockerfile
	docker build -t mittwald/servicegateway .
