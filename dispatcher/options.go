package dispatcher
import (
	"mittwald.de/servicegateway/auth"
	"mittwald.de/servicegateway/cache"
	"mittwald.de/servicegateway/ratelimit"
	"mittwald.de/servicegateway/proxy"
)

type dispatcherSetters interface {
	setAuth(auth.AuthDecorator)
	setCache(cache.CacheMiddleware)
	setRatelimit(ratelimit.RateLimitingMiddleware)
	setProxy(*proxy.ProxyHandler)
}

type DispatcherOption func (dispatcherSetters) error

func AuthHandler(auth auth.AuthDecorator) DispatcherOption {
	return func(d dispatcherSetters) error {
		d.setAuth(auth)
		return nil
	}
}

func ProxyHandler(prx *proxy.ProxyHandler) DispatcherOption {
	return func(d dispatcherSetters) error {
		d.setProxy(prx)
		return nil
	}
}

func CachingMiddleware(cch cache.CacheMiddleware) DispatcherOption {
	return func(d dispatcherSetters) error {
		d.setCache(cch)
		return nil
	}
}

func RateLimitingMiddleware(rlim ratelimit.RateLimitingMiddleware) DispatcherOption {
	return func(d dispatcherSetters) error {
		d.setRatelimit(rlim)
		return nil
	}
}