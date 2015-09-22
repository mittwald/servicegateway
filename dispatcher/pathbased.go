package dispatcher

import (
	"fmt"
	"github.com/go-zoo/bone"
	"mittwald.de/servicegateway/config"
	"mittwald.de/servicegateway/proxy"
	"net/http"
	"regexp"
	"strings"
	"github.com/op/go-logging"
)

type pathBasedDispatcher struct {
	abstractDispatcher
}

func NewPathBasedDispatcher(
	cfg *config.Configuration,
	mux *bone.Mux,
	log *logging.Logger,
	opts ...DispatcherOption,
) (*pathBasedDispatcher, error) {
	dispatcher := new(pathBasedDispatcher)
	dispatcher.cfg = cfg
	dispatcher.mux = mux
	dispatcher.log = log

	for _, opt := range opts {
		if err := opt(dispatcher); err != nil {
			return nil, err
		}
	}

	if dispatcher.prx == nil {
		dispatcher.prx = proxy.NewProxyHandler(log)
	}

	return dispatcher, nil
}

func (d *pathBasedDispatcher) RegisterApplication(name string, appCfg config.Application) error {
	routes := make(map[string]http.HandlerFunc)

	backendUrl := appCfg.Backend.Url
	if backendUrl == "" && appCfg.Backend.Service != "" {
		if appCfg.Backend.Tag != "" {
			backendUrl = fmt.Sprintf("http://%s.%s.service.consul", appCfg.Backend.Tag, appCfg.Backend.Service)
		} else {
			backendUrl = fmt.Sprintf("http://%s.service.consul", appCfg.Backend.Service)
		}
	}

	fmt.Println(appCfg.Routing)

	if appCfg.Routing.Type == "path" {
		var handler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
			sanitizedPath := strings.Replace(req.URL.Path, appCfg.Routing.Path, "", 1)
			proxyUrl := backendUrl + sanitizedPath

			d.prx.HandleProxyRequest(rw, req, proxyUrl, name, &appCfg)
		}

		routes[appCfg.Routing.Path+"/*"] = handler
	} else if appCfg.Routing.Type == "pattern" {
		re := regexp.MustCompile(":([a-zA-Z0-9]+)")
		for pattern, target := range appCfg.Routing.Patterns {
			parameters := re.FindAllStringSubmatch(pattern, -1)
			var patternHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
				targetUrl := backendUrl + target
				for _, paramName := range parameters {
					targetUrl = strings.Replace(targetUrl, paramName[0], bone.GetValue(req, paramName[1]), -1)
				}

				d.prx.HandleProxyRequest(rw, req, targetUrl, name, &appCfg)
			}

			routes[pattern] = patternHandler
		}
	}

	if d.auth != nil {
		if err := d.auth.RegisterRoutes(d.mux); err != nil {
			return err
		}
	}

	for route, handler := range routes {
		safeHandler := handler
		unsafeHandler := handler

		if d.cache != nil && appCfg.Caching.Enabled {
			safeHandler = d.cache.DecorateHandler(handler)

			if appCfg.Caching.AutoFlush {
				unsafeHandler = d.cache.DecorateUnsafeHandler(handler)
			}
		}

		if d.auth != nil && !appCfg.Auth.Disable {
			safeHandler = d.auth.DecorateHandler(safeHandler, &appCfg)
			unsafeHandler = d.auth.DecorateHandler(unsafeHandler, &appCfg)
		}

		if d.rlim != nil && appCfg.RateLimiting {
			safeHandler = d.rlim.DecorateHandler(safeHandler)
			unsafeHandler = d.rlim.DecorateHandler(unsafeHandler)
		}

		d.mux.GetFunc(route, safeHandler)
		d.mux.HeadFunc(route, safeHandler)
		d.mux.OptionsFunc(route, safeHandler)

		d.mux.PostFunc(route, unsafeHandler)
		d.mux.PutFunc(route, unsafeHandler)
		d.mux.DeleteFunc(route, unsafeHandler)
	}

	return nil
}
