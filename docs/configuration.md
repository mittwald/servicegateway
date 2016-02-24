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

Property  | Type | Description
--------- | ---- | -----------
`url` **(required if `service` is not set)** | `string` | The backend URL
`service` **(required if `url` is not set)** | `string` | The service name (must be registered with this ID as a service in Consul)
`tag`     | `string` | A service tag as registered in Consul (only when the `service` property is set)
`username` | `string` | A username to use for HTTP basic authentication at the upstream service
`password` | `string` | A password to use for HTTP basic authentication (only required when `username` is also set)
`path`    | `string` | An URL path to prepend for upstream requests (and to strip from upstream responses) -- only when the `service` property is set

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

Property     | Type   | Description
------------ | ------ | --------------------------------------------------------
`disable`    | `bool` | Set to `true` to disable authentication for this upstream service
`storage`    | [Authentication storage configuration](#Authentication storage configuration) | How the authentication token should be stored in requests made to the upstream service. See [authentication forwarding](#Authentication forwarding) for more information.

### Authentication storage configuration

Property     | Type     | Description
------------ | -------- | ------------------------------------------------------
`mode` **(required)** | `string` | One of `header`, `cookie` or `session`
`name` **(required)** | `string` | Name of the header or cookie (depending on `mode`)
`cookie_domain`       | `string` | Domain value for the set cookie. If unspecified, the current host name will be used (which might be a bad idea if you use [hostname-based dispatching](#Routing and dispatching))
`cookie_httponly`     | `bool`   | Make this cookie an HTTP-only cookie
`cookie_secure`       | `bool`   | Enforce HTTPS for this cookie

## Static configuration

The static configuration file is a JSON document consisting of the following properties:

Property         | Type   | Description
---------------- | ------ | ----------------------------------------------------
`applications`   | List of [application configs](#Application configuration) | Statically configured applications. These will be loaded *before* the ones configured in Consul and can not be overwritten at run-time
`rate_limiting`  | [Rate-limiting configuration](#Rate-limiting configuration)
`authentication` **(required)** | [Authentication configuration](#Authentication configuration)
`consul` **(required)** | [Consul configuration](#Consul configuration)
`redis` **(required)**  | `string` | Address (hostname and port) of the Redis server used for rate limiting and caching

### Rate-limiting configuration

Property                | Type   | Description
----------------------- | ------ | ---------------------------------------------
`burst` **(required)**  | `int`  | Maximum amount of allowed requests within one time window
`window` **(required)** | `string` | A [duration specifier](go-duration) for the length of the time window after which the rate limit is reset

### Authentication configuration

Property         | Type     | Description
---------------- | -------- | --------------------------------------------------
`mode` **(required)** | `string` | Either `rest` or `graphical`
`storage` **(required)** | [Authentication storage configuration](#Authentication storage configuration) | How the authentication token is expected to be stored in client requests. See [authentication forwarding](#Authentication forwarding) for more information.
`provider` **(required)** | [Authentication provider configuration](#Authentication provider configuration)
`verification_key` **(required if `verification_key_url` is not set)** | `string` | The secret key used to authenticate JWTs of incoming requests
`verification_key_url` **(required if `verification_key` is not set)** | `string` | The URL of the secret key used to authenticate JWTs of incoming requests
`key_cache_ttl` | `string` | A [duration specifier](go-duration) describing for how long the verification key should be cached

### Authentication provider configuration

Property         | Type     | Description
---------------- | -------- | --------------------------------------------------
`url` **(required)** | `string` | The URL of the authentication endpoint
`parameters`     | arbitrary JSON | Only of interest when you're using the `graphical` authentication mode and using the built-in login form. This option contains a base JSON document with the parameters that will be used as authentication request. After submitting the built-in login form, a `username` and `password` parameter will be added to this parameter set

### Consul configuration

Property         | Type     | Description
---------------- | -------- | --------------------------------------------------
`host`           | `string` | The Consul host name
`port`           | `int`    | The port of Consul's REST API (typically `8500`)