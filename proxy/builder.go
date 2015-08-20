package proxy

import (
	"net/http"
	"github.com/go-zoo/bone"
	"mittwald.de/charon/config"
	"strings"
	"regexp"
	"fmt"
)

type ProxyBuilder struct {
	ProxyHandler *ProxyHandler
	Cache *Cache
}

func NewProxyBuilder(proxy *ProxyHandler, cache *Cache) *ProxyBuilder {
	b := new(ProxyBuilder)
	b.ProxyHandler = proxy
	b.Cache = cache
	return b
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
			fmt.Println(parameters)
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

		mux.GetFunc(route, safeHandler)
		mux.HeadFunc(route, safeHandler)
		mux.OptionsFunc(route, safeHandler)

		mux.PostFunc(route, unsafeHandler)
		mux.PutFunc(route, unsafeHandler)
		mux.DeleteFunc(route, unsafeHandler)
	}
}
