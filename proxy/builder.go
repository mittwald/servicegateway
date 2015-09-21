package proxy

import (
	"net/http"
	"github.com/go-zoo/bone"
	"mittwald.de/servicegateway/config"
	"github.com/mailgun/oxy/forward"
	"github.com/mailgun/oxy/testutils"
	"github.com/mailgun/oxy/roundrobin"
	consul "github.com/hashicorp/consul/api"
	"strings"
	"regexp"
	"fmt"
	"mittwald.de/servicegateway/auth"
	"net/url"
	"strconv"
)

type ProxyBuilder struct {
	Configuration *config.Configuration
	ProxyHandler *ProxyHandler
	Cache *Cache
	RateThrottler *RateThrottler
	AuthDecorator auth.AuthDecorator
	Consul *consul.Client
}

func NewProxyBuilder(cfg *config.Configuration, proxy *ProxyHandler, cache *Cache, throttler *RateThrottler, authDecorator auth.AuthDecorator) (*ProxyBuilder, error) {
	consulConfig := consul.DefaultConfig()
	consulConfig.Address = cfg.Consul.URL

	consul, err := consul.NewClient(consulConfig)
	if err != nil {
		return err
	}

	return &ProxyBuilder {
		Configuration: cfg,
		ProxyHandler: proxy,
		Cache: cache,
		RateThrottler: throttler,
		AuthDecorator: authDecorator,
		Consul: consul,
	}
}

func debugHandlerDecorator(app string, handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("X-Charon-TargetApplication", app)
		handler(rw, req)
	}
}

func (b *ProxyBuilder) BuildHandler(mux *bone.Mux, name string, appCfg config.Application) error {
	routes := make(map[string]http.HandlerFunc)

	forwarderConfig := func (fwd *forward.Forwarder) error {
		return nil
	}
	forwarder, err := forward.New(forwarderConfig)
	if err != nil {
		return err
	}
	lb, _ := roundrobin.New(forwarder)

	services, _, err := b.Consul.Catalog().Service(appCfg.Backend.Service, appCfg.Backend.Tag)
	if err != nil {
		return err
	}

	for _, service := range(services) {
		lb.UpsertServer(&url.URL{
			Scheme: "http",
			Host: service.Address + ":" + strconv.Itoa(service.ServicePort),
		})
	}

//	lb.UpsertServer(testutils.ParseURI(appCfg.Backend.Url))

	if appCfg.Routing.Type == "path" {
		var handler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
			sanitizedPath := strings.Replace(req.URL.Path, appCfg.Routing.Path, "", 1)
			proxyUrl := appCfg.Backend.Url + sanitizedPath
			req.URL = testutils.ParseURI(proxyUrl)

			lb.ServeHTTP(rw, req)
			//b.ProxyHandler.HandleProxyRequest(rw, req, proxyUrl, name, &appCfg)
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

				req.URL = testutils.ParseURI(targetUrl)
				lb.ServeHTTP(rw, req)
//				b.ProxyHandler.HandleProxyRequest(rw, req, targetUrl, name, &appCfg)
			}

			routes[pattern] = patternHandler
		}
	}

	if err := b.AuthDecorator.RegisterRoutes(mux); err != nil {
		return err
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

	return nil
}
