package proxy

import (
	"net/http"
	"github.com/go-zoo/bone"
	"mittwald.de/servicegateway/config"
	"strings"
	"regexp"
	"fmt"
	"mittwald.de/servicegateway/auth"
)

type ProxyBuilder struct {
	Configuration *config.Configuration
	ProxyHandler *ProxyHandler
	Cache *Cache
	RateThrottler *RateThrottler
	AuthDecorator auth.AuthDecorator
}

func NewProxyBuilder(cfg *config.Configuration, proxy *ProxyHandler, cache *Cache, throttler *RateThrottler, authDecorator auth.AuthDecorator) *ProxyBuilder {
	return &ProxyBuilder {
		Configuration: cfg,
		ProxyHandler: proxy,
		Cache: cache,
		RateThrottler: throttler,
		AuthDecorator: authDecorator,
	}
}

func debugHandlerDecorator(app string, handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("X-Charon-TargetApplication", app)
		handler(rw, req)
	}
}

func (b *ProxyBuilder) BuildHandler(mux *bone.Mux, name string, appCfg config.Application) {
	routes := make(map[string]http.HandlerFunc)

	if appCfg.Routing.Type == "path" {
		var handler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
			sanitizedPath := strings.Replace(req.URL.Path, appCfg.Routing.Path, "", 1)
			proxyUrl := appCfg.Backend.Url + sanitizedPath
			b.ProxyHandler.HandleProxyRequest(rw, req, proxyUrl, name, &appCfg)
		}

		routes[appCfg.Routing.Path+"/*"] = handler
	} else if appCfg.Routing.Type == "pattern" {
		re := regexp.MustCompile(":([a-zA-Z0-9]+)")
		for pattern, target := range appCfg.Routing.Patterns {
			parameters := re.FindAllStringSubmatch(pattern, -1)
			var patternHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
				targetUrl := appCfg.Backend.Url + target
				for _, paramName := range parameters {
					targetUrl = strings.Replace(targetUrl, paramName[0], bone.GetValue(req, paramName[1]), -1)
				}
				fmt.Println(targetUrl)

				b.ProxyHandler.HandleProxyRequest(rw, req, targetUrl, name, &appCfg)
			}

			routes[pattern] = patternHandler
		}
	}

	for route, handler := range routes {
		safeHandler := handler
		unsafeHandler := handler

		if appCfg.Caching.Enabled {
			safeHandler = b.Cache.DecorateHandler(handler)

			if appCfg.Caching.AutoFlush {
				unsafeHandler = b.Cache.DecorateUnsafeHandler(handler)
			}
		}

		if ! appCfg.Auth.Disable {
			safeHandler = b.AuthDecorator.DecorateHandler(safeHandler)
			unsafeHandler = b.AuthDecorator.DecorateHandler(unsafeHandler)
		}

		if appCfg.RateLimiting {
			safeHandler = b.RateThrottler.DecorateHandler(safeHandler)
			unsafeHandler = b.RateThrottler.DecorateHandler(unsafeHandler)
		}

		mux.GetFunc(route, safeHandler)
		mux.HeadFunc(route, safeHandler)
		mux.OptionsFunc(route, safeHandler)

		mux.PostFunc(route, unsafeHandler)
		mux.PutFunc(route, unsafeHandler)
		mux.DeleteFunc(route, unsafeHandler)
	}
}
