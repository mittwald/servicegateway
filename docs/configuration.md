# Configuration reference

## Application configuration

Each application configuration is a JSON document adhering to the following schema:

Property                 | Type
------------------------ | -----------------------------------------------
`backend` **(required)** | [Backend configuration](#Backend configuration)
`routing` **(required)** | [Routing configuration](#Routing configuration)
`caching`                | [Caching configuration](#Caching configuration) or empty (not specifying this value will disable caching)
`auth`                   | [Authentication configuration](#Application authentication configuration) or empty (if unspecified, authentication will be required by the gateway, but not forwarded to the upstream service)
`rate_limiting`          | `true`, `false` or empty (`false` if unspecified)

### Backend configuration

A backend configuration must consist of **either** a `url` property or a `service` property. They are mutually exclusive.

Property   | Type     | Description
---------- | -------- | -----------
`url` **(required if `service` is not set)** | `string` | The backend URL
`service` **(required if `url` is not set)** | `string` | The service name (must be registered with this ID as a service in Consul)
`tag`      | `string` | A service tag as registered in Consul (only when the `service` property is set)
`username` | `string` | A username to use for HTTP basic authentication at the upstream service
`password` | `string` | A password to use for HTTP basic authentication (only required when `username` is also set)
`path`     | `string` | An URL path to prepend for upstream requests (and to strip from upstream responses) -- only when the `service` property is set

### Routing configuration

Property | Type | Description
-------- | ---- | -----------
`type` **(required)** | `string` | One of `hostname`, `path` or `pattern`. See [Routing and Dispatching](#Routing and Dispatching) for more information
`hostname` **(required if `type` is `hostname`)** | `string` | Requests with this hostname (HTTP `Host` header) will be routed to this upstream application
`path` **(required if `type` is `path`)** | `string` | Requests with this path prefix will be routed to this upstream application
`patterns` **(required if `type` is `pattern`)** | `map[string]string` | A map of request patterns (formatted like `foo/bar/:param`), using incoming request patterns as key and outgoing patterns as value.

### Caching configuration

Property     | Type   | Description
------------ | ------ | --------------------------------------------------------
`enabled`    | `bool` | Set to `true` to enable caching
`ttl`        | `int`  | Default time-to-live in seconds
`auto_flush` | `bool` | Automatically flush the cache if a non-GET request is sent to the same URI (really useful for really RESTful webservices)

### Application authentication configuration

Property  | Type   | Description
--------- | ------ | --------------------------------------------------------
`disable` | `bool` | Set to `true` to disable authentication for this upstream service
`writer`  | [Authentication writer configuration](#Authentication writer configuration) | How the authentication token should be written in requests made to the upstream service. See [authentication forwarding](#Authentication forwarding) for more information.

### Authentication writer configuration

Property     | Type     | Description
------------ | -------- | ------------------------------------------------------
`mode` **(required)** | `string` | One of `header` or `authorization`
`name` **(required)** | `string` | Name of the header (depending on `mode`)

## Static configuration

The static configuration file is a JSON document consisting of the following properties:

Property         | Type   | Description
---------------- | ------ | ----------------------------------------------------
`applications`   | List of [application configs](#Application configuration) | Statically configured applications. These will be loaded *before* the ones configured in Consul and can not be overwritten at run-time
`rate_limiting`  | [Rate-limiting configuration](#Rate-limiting configuration)
`authentication` **(required)** | [Authentication configuration](#Authentication configuration)
`consul` **(required)** | [Consul configuration](#Consul configuration)
`redis` **(required)**  | [Redis backend configuration](#Redis backend configuration) | Address (hostname and port) of the Redis server used for rate limiting and caching
`proxy` | [HTTP proxy configuration](#HTTP proxy configuration) | HTTP proxy configuration

### Rate-limiting configuration

Property                | Type   | Description
----------------------- | ------ | ---------------------------------------------
`burst` **(required)**  | `int`  | Maximum amount of allowed requests within one time window
`window` **(required)** | `string` | A [duration specifier](go-duration) for the length of the time window after which the rate limit is reset

### Authentication configuration

Property         | Type     | Description
---------------- | -------- | --------------------------------------------------
`mode` **(required)** | `string` | Currently, only `mapping` is supported
`provider` **(required)** | [Authentication provider configuration](#Authentication provider configuration)
`verification_key` **(required if `verification_key_url` is not set)** | `string` | The secret key used to authenticate JWTs of incoming requests
`verification_key_url` **(required if `verification_key` is not set)** | `string` | The URL of the secret key used to authenticate JWTs of incoming requests
`key_cache_ttl` | `string` | A [duration specifier](go-duration) describing for how long the verification key should be cached

### Authentication provider configuration

Property         | Type     | Description
---------------- | -------- | --------------------------------------------------
`url` **(required)** | `string` | The URL of the authentication endpoint. Currently, not used.

### Consul configuration

Property         | Type     | Description
---------------- | -------- | --------------------------------------------------
`host`           | `string` | The Consul host name
`port`           | `int`    | The port of Consul's REST API (typically `8500`)

### HTTP proxy configuration

Property            | Type                | Description
------------------- | ------------------- | --------------------------------------------------
`strip_res_headers` | `map[string]bool`   | Headers to strip from upstream response
`set_res_headers`   | `map[string]string` | Headers that should be added to the HTTP response
`set_req_headers`   | `map[string]string` | Headers to add to the upstream request
