[![Build Status](https://travis-ci.org/mittwald/servicegateway.svg?branch=master)](https://travis-ci.org/mittwald/servicegateway)
[![Go Report Card](https://goreportcard.com/badge/github.com/mittwald/servicegateway)](https://goreportcard.com/report/github.com/mittwald/servicegateway)
[![Docker Repository on Quay](https://quay.io/repository/mittwald/servicegateway/status "Docker Repository on Quay")](https://quay.io/repository/mittwald/servicegateway)

# Service gateway for microservice architectures

## Author and license

Martin Helmich  
Mittwald CM Service GmbH & Co. KG

This code is [GPL-licensed](LICENSE.txt).

## Synopsis

This repository contains a service gateway that can be used both as a
single-signon gateway or API gateway for a [microservice architecture][fowler-microservices].
It implements features like load balancing, rate limiting and (rudimentary) HTTP
caching. It uses [Consul][consul] for service discovery and configuration
management.

## Compilation and installation

For building, you will need a halfway current [Go SDK][go] (tested with 1.12). Then simply `go install`:

```shellsession
> go install github.com/mittwald/servicegateway
```

This will produce a (more or less) static binary that you can deploy without any
dependencies.

Alternatively, use the Makefile that is shipped within this repository to build
the binary and/or a [Docker][docker] image containing this application:

```shellsession
> make
> make docker
```

## Configuration

### Configuration sources

#### Basic configuration file

The basic configuration is read from a configuration file that by default is
expected to be located in `/etc/servicegateway.json`. However, you can override
that location using the `-config` command line parameter.

Check the [example-configs](example-configs) directory for example
configurations.

#### Configuration with Consul

Most of the configuration options (that are not required for the actual program
startup) can also be provided by [Consul][consul], an open-source service
discovery engine. Configuration can be stored in Consul's [key-value store][consul-kv].

Consul uses a hierarchical key-value store. All configuration items for the
service gateway must be stored under a common key prefix that is supplied via
the `-consul-base` command-line parameter. This affects the following
configuration items:

1.  Rate-limiting configuration (key `<base-prefix>/ratelimiting`)
2.  Caching configuration (key `<base-prefix>/caching`)
3.  Upstream application (keys `<base-prefix>/applications/<app-identifier>`)

Each upstream application is its own key/value pair with the value being a JSON
document describing the application.

You can configure the key prefix in which the service gateway should look for
configured applications using the `-consul-base` parameter:

    ./servicegateway -consul-base gateway/applications

You can create a new application like follows (substitute `<name>` with an
arbitrary identifier for your application):

```shellsession
> curl -X PUT -d @app.json http://localhost:8500/v1/kv/gateway/applications/<name>
```

This `PUT`s the contents of the following file `app.json` into the Consul
key/value store:

```json
{
  "backend": {
    "url": "http://httpbin.org"
  },
  "caching": {
    "auto_flush": true,
    "enabled": true,
    "ttl": 3600
  },
  "routing": {
    "type": "path",
    "path": "/bin"
  },
  "rate_limiting": true
}
```

**Important**: Configuration changes made in Consul will become effective
immediately, without needing to restart the service gateway.

### Configuration reference

See the [documentation reference](docs/configuration.md).

## Core concepts

### Routing and dispatching

For this service gateway, multiple upstream applications can be configured. Each
application is a key/value entry in Consul's key/value store.

The servicegateway employs a compex logic to determine which HTTP request to
route to which upstream application. Currently, there are three different
strategies supported that can be used alongside each other:

-   **Path based routing**: The target upstream application is determined by a
    HTTP path prefix. For example, all requests having a path starting with
    `/one` may be routed to one upstream application and all requests starting
    with `/two`to another upstream application. This may cause issues when the
    response from the upstream applications contain absolute links to other
    documents (like in-document links, `Link` headers or `Location` headers);
    the service gateway tries to rewrite these links to use the path prefix
    configured for the upstream application.

    Example:

    ```json
    {
      "type": "path",
      "path": "/one"
    }
    ```

-   **Host based routing**: The target upstream application is determined by
    the HTTP host header.

    Example:

    ```json
    {
      "type": "host",
      "host": "name.servcices.acme.corp"
    }
    ```

-   **Pattern based routing**: This is the most complex routing strategy. For
    each application, you can configure a set of path patterns that are mapped
    to path patterns of the upstream application.

    Example:

    ```json
    {
      "type": "pattern",
      "patterns": {
        "/products": "app.php?controller=products&action=list",
        "/products/:id": "app.php?controller=products&action=show&product_id=:id"
      }
    }
    ```

Applications can be configured by adding new key/value entries into Consul's
key/value store under the configured prefix. This can be done at runtime;
changes become effective immediately without restarting the servicegateway.

### Authentication forwarding

The Servicegateway also features a (very opinionated) authentication handling.
Currently, upstream services are expected to authenticate users using
[JSON Web Tokens][jwt]. However, the service gateway will not expose these JWTs
to the end user. Instead, it is built to map JWTs to random and non-informative
API tokens; when receiving a request with an API token, the gateway will lookup
the respective JWT from a Redis storage and attach it to the request to the
upstream service.

#### Basic configuration

In order to make authentication work, you'll need the following:

1.  Configure the key that the gateway can use to verify tokens presented by
    users. For this, you can either specify the key directly, or specify a URL
    from which the key can be loaded.

    ```json
    {
      "authentication": {
        "verification_key": "...",
        "verification_key_url": "..."
      }
    }
    ```

2.  Configure how to add mapped JWTs to the upstream service requests.
    Currently, the token can be included in a custom header or an
    `Authorization` header.

    ```json
    {
      "authentication": {
        "mode": "rest",
        "writer": {
          "mode": "header",
          "name": "X-Jwt"
        }
      }
    }
    ```

#### Adding new tokens

The service gateway implementes an administration API that listens on a
different port than the actual gateway (**caution**: the administration port
does not provide authentication, do not expose it to the public!); you can use
this to pre-configure access tokens for users.

```shellsession
> curl -X POST -H 'Content-Type: application/jwt' -d 'JWT contents...' http://localhost:8081/tokens
{"token":"DLOD5FCRO6PVSLVWD7QPPGIIBXK7XXFACV7LMKEUZOP6DCADXTSQ===="}
```

Using the same admin API, you can also update existing tokens (for example, if
they contained an expirable JWT):

```shellsession
> curl -X PUT -H 'Content-Type: application/jwt' -d 'JWT contents...' http://localhost:81/tokens/DLOD5FCRO6PVSLVWD7QPPGIIBXK7XXFACV7LMKEUZOP6DCADXTSQ%3D%3D%3D%3D
{"token":"DLOD5FCRO6PVSLVWD7QPPGIIBXK7XXFACV7LMKEUZOP6DCADXTSQ===="}
```

[consul]: https://consul.io
[consul-kv]: https://www.consul.io/docs/agent/http/kv.html
[docker]: https://www.docker.com
[fowler-microservices]: http://martinfowler.com/articles/microservices.html
[go]: https://golang.org/dl/
[go-duration]: https://golang.org/pkg/time/#ParseDuration
[jwt]: http://jwt.io/
