package dispatcher

/*
 * Microservice gateway application
 * Copyright (C) 2015  Martin Helmich <m.helmich@mittwald.de>
 *                     Mittwald CM Service GmbH & Co. KG
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

import (
	"fmt"
	"github.com/go-zoo/bone"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/op/go-logging"
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

	var rewriter proxy.HostRewriter

	if appCfg.Routing.Type == "path" {
		mapping := map[string]string{
			"/(.*)": appCfg.Routing.Path + "/$1",
		}

		rewriter, _ = proxy.NewHostRewriter(backendUrl, "foobar", []string{}, mapping, d.log)

		var handler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
			sanitizedPath := strings.Replace(req.URL.Path, appCfg.Routing.Path, "", 1)
			proxyUrl := backendUrl + sanitizedPath

			d.prx.HandleProxyRequest(rw, req, proxyUrl, name, &appCfg)
		}

		routes[appCfg.Routing.Path+"/*"] = http.HandlerFunc(handler)
	} else if appCfg.Routing.Type == "pattern" {
		re := regexp.MustCompile(":([a-zA-Z0-9]+)")
		mapping := make(map[string]string)

		for pattern, target := range appCfg.Routing.Patterns {
			targetPattern := "^" + re.ReplaceAllString(target, "(?P<$1>[^/]+?)") + "$"
			mapping[targetPattern] = pattern

			parameters := re.FindAllStringSubmatch(pattern, -1)
			var patternHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
				targetUrl := backendUrl + target
				for _, paramName := range parameters {
					targetUrl = strings.Replace(targetUrl, paramName[0], bone.GetValue(req, paramName[1]), -1)
				}

				d.prx.HandleProxyRequest(rw, req, targetUrl, name, &appCfg)
			}

			fmt.Println(mapping)
			routes[pattern] = http.HandlerFunc(patternHandler)
		}

		rewriter, _ = proxy.NewHostRewriter(backendUrl, "foobar", []string{}, mapping, d.log)
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
		handler = rewriter.Decorate(handler)

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
