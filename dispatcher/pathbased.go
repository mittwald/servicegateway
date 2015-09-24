package dispatcher

import (
	"fmt"
	"github.com/go-zoo/bone"
	"github.com/op/go-logging"
	"mittwald.de/servicegateway/config"
	"mittwald.de/servicegateway/proxy"
	"net/http"
	"regexp"
	"strings"
)

type pathBasedDispatcher struct {
	abstractDispatcher
}

func NewPathBasedDispatcher(
	cfg *config.Configuration,
	log *logging.Logger,
	prx *proxy.ProxyHandler,
) (*pathBasedDispatcher, error) {
	dispatcher := new(pathBasedDispatcher)
	dispatcher.cfg = cfg
	dispatcher.mux = bone.New()
	dispatcher.log = log
	dispatcher.prx = prx
	dispatcher.behaviours = make([]DispatcherBehaviour, 0, 8)

	return dispatcher, nil
}

func (d *pathBasedDispatcher) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	d.mux.ServeHTTP(res, req)
}

func (d *pathBasedDispatcher) RegisterApplication(name string, appCfg config.Application) error {
	routes := make(map[string]http.Handler)

	backendUrl := appCfg.Backend.Url
	if backendUrl == "" && appCfg.Backend.Service != "" {
		if appCfg.Backend.Tag != "" {
			backendUrl = fmt.Sprintf("http://%s.%s.service.consul", appCfg.Backend.Tag, appCfg.Backend.Service)
		} else {
			backendUrl = fmt.Sprintf("http://%s.service.consul", appCfg.Backend.Service)
		}
	}

	if appCfg.Routing.Type == "path" {
		var handler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
			sanitizedPath := strings.Replace(req.URL.Path, appCfg.Routing.Path, "", 1)
			proxyUrl := backendUrl + sanitizedPath

			d.prx.HandleProxyRequest(rw, req, proxyUrl, name, &appCfg)
		}

		routes[appCfg.Routing.Path+"/*"] = http.HandlerFunc(handler)
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

			routes[pattern] = http.HandlerFunc(patternHandler)
		}
	}

	for _, behaviour := range d.behaviours {
		switch t := behaviour.(type) {
		case RoutingBehaviour:
			if err := t.AddRoutes(d.mux); err != nil {
				return err
			}
		}
	}

	for route, handler := range routes {
		safeHandler := handler
		unsafeHandler := handler

		for _, behaviour := range d.behaviours {
			var err error
			safeHandler, unsafeHandler, err = behaviour.Apply(safeHandler, unsafeHandler, d, &appCfg)
			if err != nil {
				return err
			}
		}

		d.mux.Get(route, safeHandler)
		d.mux.Head(route, safeHandler)
		d.mux.Options(route, safeHandler)
		d.mux.Post(route, unsafeHandler)
		d.mux.Put(route, unsafeHandler)
		d.mux.Delete(route, unsafeHandler)
	}

	return nil
}

func (d *pathBasedDispatcher) Initialize() error {
	return nil
}
