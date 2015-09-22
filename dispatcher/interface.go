package dispatcher

import (
	"net/http"
	"mittwald.de/servicegateway/config"
	"github.com/go-zoo/bone"
	"mittwald.de/servicegateway/proxy"
	"github.com/op/go-logging"
	"mittwald.de/servicegateway/auth"
	"mittwald.de/servicegateway/cache"
	"mittwald.de/servicegateway/ratelimit"
)

type Dispatcher interface {
	http.Handler
	RegisterApplication(string, config.Application) error
}

type abstractDispatcher struct {
	cfg *config.Configuration
	mux *bone.Mux
	prx *proxy.ProxyHandler
	log *logging.Logger

	auth auth.AuthDecorator
	cache cache.CacheMiddleware
	rlim ratelimit.RateLimitingMiddleware
}

func (d *abstractDispatcher) setAuth(a auth.AuthDecorator) {
	d.auth = a
}

func (d *abstractDispatcher) setCache(c cache.CacheMiddleware) {
	d.cache = c
}

func (d *abstractDispatcher) setRatelimit(r ratelimit.RateLimitingMiddleware) {
	d.rlim = r
}

func (d *abstractDispatcher) setProxy(p *proxy.ProxyHandler) {
	d.prx = p
}
