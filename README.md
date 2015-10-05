# Service gateway for microservice architectures

## Author and license

Martin Helmich  
Mittwald CM Service GmbH & Co. KG

This code is [GPL-licensed](LICENSE.txt).

## Synopsis

This repository contains a service gateway that can be used both as a single-signon gateway or API gateway for a [microservice architecture][fowler-microservices]. It implements features like load balancing, rate limiting and (rudimentary) HTTP caching. It uses [Consul][consul] for service discovery and configuration management.

## Compilation and installation

For building, you will need a halfway current [Go SDK][go] (tested with 1.4 and 1.5). Then simply `go install`:

```shellsession
> go install github.com/mittwald/servicegateway
```

This will produce a (more or less) static binary that you can deploy without any dependencies.

Alternatively, use the Dockerfile that is shipped within this repository to build a [Docker][docker] image containing this application:

```shellsession
> docker build -t servicegateway .
```

## Configuration

### Configuration sources

#### Basic configuration file

The basic configuration is read from a configuration file that by default is expected to be located in `/etc/servicegateway.json`. However, you can override that location using the `-config` command line parameter.

Check the [example-configs](example-configs) directory for example configurations for usage as API gateway and Single-SignOn gateway.

#### Configuration with Consul

Most of the configuration options (that are not required for the actual program startup) can also be provided by [Consul][consul], an open-source service discovery engine. Configuration can be stored in Consul's [key-value store][consul-kv].

Consul uses a hierarchical key-value store. All configuration items for the servicegateway must be stored under a common key prefix that is supplied via the `-consul-base` command-line parameter.
This affects the following configuration items:

1.  Rate-limiting configuration (key `<base-prefix>/ratelimiting`)
2.  Caching configuration (key `<base-prefix>/caching`)
3.  Upstream application (keys `<base-prefix>/application/<app-identifier>`)

Each upstream application is its own key/value pair with the value being a JSON document describing the application.

You can configure the key prefix in which the service gateway should look for configured applications using the `-consul-base` parameter:

    ./servicegateway -consul-base gateway/applications

You can create a new application like follows (substitute `<name>` with an arbitrary identifier for your application):

```shellsession
> curl -X PUT -d @app.json http://localhost:8500/v1/kv/gateway/applications/<name>
```

This `PUT`s the contents of the following file `app.json` into the Consul key/value store:

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

**Important**: Configuration changes made in Consul will become effective immediately, without needing to restart the service gateway.

### Configuration reference

See the [documentation reference](docs/configuration.md).

## Core concepts

### Routing and dispatching

tbw.

### Authentication forwarding

tbw.

[consul]: https://consul.io
[consul-kv]: https://www.consul.io/docs/agent/http/kv.html
[docker]: https://www.docker.com
[fowler-microservices]: http://martinfowler.com/articles/microservices.html
[go]: https://golang.org/dl/
[go-duration]: https://golang.org/pkg/time/#ParseDuration
