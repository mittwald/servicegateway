package dispatcher
import (
	"mittwald.de/servicegateway/ratelimit"
	"net/http"
	"mittwald.de/servicegateway/config"
	"mittwald.de/servicegateway/cache"
	"mittwald.de/servicegateway/auth"
	"github.com/go-zoo/bone"
)

type cachingBehaviour struct {
	cache cache.CacheMiddleware
}

type authBehaviour struct {
	auth auth.AuthDecorator
}

type ratelimitBehaviour struct {
	rlim ratelimit.RateLimitingMiddleware
}

func NewCachingBehaviour(c cache.CacheMiddleware) (DispatcherBehaviour) {
	return &cachingBehaviour{c}
}

func (c *cachingBehaviour) Apply(safe http.Handler, unsafe http.Handler, d Dispatcher, app *config.Application) (http.Handler, http.Handler, error) {
	if app.Caching.Enabled {
		safe = c.cache.DecorateHandler(safe)

		if app.Caching.AutoFlush {
			unsafe = c.cache.DecorateUnsafeHandler(unsafe)
		}
	}
	return safe, unsafe, nil
}

func NewAuthenticationBehaviour(a auth.AuthDecorator) (DispatcherBehaviour) {
	return &authBehaviour{a}
}

func (a *authBehaviour) Apply(safe http.Handler, unsafe http.Handler, d Dispatcher, app *config.Application) (http.Handler, http.Handler, error) {
	if !app.Auth.Disable {
		safe = a.auth.DecorateHandler(safe, app)
		unsafe = a.auth.DecorateHandler(unsafe, app)
	}
	return safe, unsafe, nil
}

func (a *authBehaviour) AddRoutes(mux *bone.Mux) error {
	return a.auth.RegisterRoutes(mux)
}

func NewRatelimitBehaviour(rlim ratelimit.RateLimitingMiddleware) (DispatcherBehaviour) {
	return &ratelimitBehaviour{rlim}
}

func (r *ratelimitBehaviour) Apply(safe http.Handler, unsafe http.Handler, d Dispatcher, app *config.Application) (http.Handler, http.Handler, error) {
	if app.RateLimiting {
		safe = r.rlim.DecorateHandler(safe)
		unsafe = r.rlim.DecorateHandler(unsafe)
	}
	return safe, unsafe, nil
}