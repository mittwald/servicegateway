FROM        alpine:3.12
LABEL       MAINTAINER="Martin Helmich <m.helmich@mittwald.de>"
COPY        servicegateway /usr/bin/servicegateway
RUN         apk add --no-cache --upgrade ca-certificates && \
            adduser servicegateway -D -h / -s /sbin/nologin && \
            mkdir -p /usr/share/servicegateway/templates && \
            mkdir -p /usr/share/servicegateway/static && \
            rm -rf /var/cache/* /tmp/*
USER        servicegateway
EXPOSE      8080
ENTRYPOINT  ["servicegateway"]