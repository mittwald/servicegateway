FROM alpine:3.2
MAINTAINER Martin Helmich <m.helmich@mittwald.de>

EXPOSE 8080

ADD servicegateway /usr/bin/servicegateway
ADD templates /usr/share/servicegateway/templates
ADD static /usr/share/servicegateway/static
RUN adduser servicegateway -D -h / -s /sbin/nologin
USER servicegateway

ENTRYPOINT ["/usr/bin/servicegateway"]
