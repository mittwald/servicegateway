FROM        alpine:3.10
LABEL       MAINTAINER="Martin Helmich <m.helmich@mittwald.de>"
COPY        servicegateway /usr/bin/servicegateway
RUN         adduser servicegateway -D -h / -s /sbin/nologin && \
            mkdir -p /usr/share/servicegateway/templates && \
            mkdir -p /usr/share/servicegateway/static
USER        servicegateway
EXPOSE      8080
ENTRYPOINT  ["servicegateway"]