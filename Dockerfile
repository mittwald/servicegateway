FROM golang:1.12-stretch AS builder
ENV CGO_ENABLED=0 \
    GOOS=linux
WORKDIR /src
COPY . .
RUN go build -o servicegateway

FROM alpine:3.8
MAINTAINER Martin Helmich <m.helmich@mittwald.de>

EXPOSE 8080

COPY --from=builder /src/servicegateway /usr/bin/servicegateway
RUN adduser servicegateway -D -h / -s /sbin/nologin && \
    mkdir -p /usr/share/servicegateway/templates && \
    mkdir -p /usr/share/servicegateway/static
USER servicegateway

ENTRYPOINT ["/usr/bin/servicegateway"]
