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

Alternatively, use the Makefile that is shipped within this repository to build the binary and/or a [Docker][docker] image containing this application:

```shellsession
> make
> make docker
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

For this service gateway, multiple upstream applications can be configured. Each application is a key/value entry in Consul's key/value store.

The servicegateway employs a compex logic to determine which HTTP request to route to which upstream application. Currently, there are three different strategies supported that can be used alongside each other:

-   **Path based routing**: The target upstream application is determined by a HTTP path prefix. For example, all requests having a path starting with `/one` may be routed to one upstream application and all requests starting with `/two` to another upstream application. This may cause issues when the response from the upstream applications contain absolute links to other documents (like in-document links, `Link` headers or `Location` headers); the servicegateway tries to rewrite these links to use the path prefix configured for the upstream application.

    Example:

    ```json
    {
      "type": "path",
      "path": "/one"
    }
    ```

-   **Host based routing**: The target upstream application is determined by the HTTP host header.

    Example:

    ```json
    {
      "type": "host",
      "host": "name.servcices.acme.corp"
    }
    ```

-   **Pattern based routing**: This is the most complex routing strategy. For each application, you can configure a set of path patterns that are mapped to path patterns of the upstream application.

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

Applications can be configured by adding new key/value entries into Consul's key/value store under the configured prefix. This can be done at runtime; changes become effective immediately without restarting the servicegateway.

### Authentication forwarding

The Servicegateway also features a (very opinionated) authentication handling. Currently, authenticaiton is done using [JSON Web Tokens][jwt] that can be sent in a request header, or a cookie. If you do not want to expose the JWT's to your users, the Servicegateway also supports traditional sessions.

#### Basic configuration

In order to make authentication work, you'll need the following:

1.   Configure the key that the gateway can use to verify tokens presented by users. For this, you can either specify the key directly, or specify a URL from which the key can be loaded.

     ```json
     {
       "authentication": {
         "verification_key": "...",
         "verification_key_url": "..."
       }
     }
     ```

2.  Configure where to look for tokens in the client requests. Currently, the token can be included in a custom header or a cookie.

     ```json
     {
       "authentication": {
         "mode": "rest",
         "storage": {
           "mode": "header",
           "name": "X-Jwt"
         }
       }
     }
     ```

3.  Configure an authentication provider that you'll redirect users to that are not authenticated.

    ```json
    {
      "authentication": {
        "mode": "rest",
        "provider": {
          "url": "https://identity.service.consul/authenticate"
        }
      }
    }
    ```

#### With graphical login form

You can also use the Servicegateway to present a graphical login form to your users (the *graphical* authentication mode). In graphical authentication mode, unauthenticated users will be presented a login form in which they can enter a username and password. Upon submit, these credentials will be submitted to the configured identity provider URL as a JSON document. The response document MUST contain a JWT that will then be stored in a cookie.

Example configuration:

```json
{
  "authentication": {
    "mode": "graphical",
    "provider": {
      "url": "https://identity.service.consul/authenticate"
    },
    "verification_key_url": "https://identity.service.consul/key",
    "storage": {
      "mode": "session",
      "name": "ACME_SESSION",
      "cookie_domain": ".services.acme.corp",
      "cookie_httponly": true,
      "cookie_secure": true
    }
  }
}
```

[consul]: https://consul.io
[consul-kv]: https://www.consul.io/docs/agent/http/kv.html
[docker]: https://www.docker.com
[fowler-microservices]: http://martinfowler.com/articles/microservices.html
[go]: https://golang.org/dl/
[go-duration]: https://golang.org/pkg/time/#ParseDuration
[jwt]: http://jwt.io/
