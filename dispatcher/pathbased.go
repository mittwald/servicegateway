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
	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

type abstractPathBasedDispatcher struct {
	abstractDispatcher
}

type PatternClosure struct {
	targetUrl  string
	parameters [][]string
	appName    string
	appCfg     *config.Application
	proxy      *proxy.ProxyHandler
}

type PathClosure struct {
	backendUrl string
	appName    string
	appCfg     *config.Application
	proxy      *proxy.ProxyHandler
}

func (d *abstractPathBasedDispatcher) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	//	for k, v := range d.cfg.Proxy.SetResponseHeaders {
	//		res.Header.Set(k, v)
	//	}

	d.mux.ServeHTTP(res, req)
}

func (p *PatternClosure) Handle(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
	targetUrl := p.targetUrl
	for _, paramName := range p.parameters {
		targetUrl = strings.Replace(targetUrl, paramName[0], params.ByName(paramName[1]), -1)
	}

	p.proxy.HandleProxyRequest(rw, req, targetUrl, p.appName, p.appCfg)
}

func (p *PathClosure) Handle(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
	sanitizedPath := strings.Replace(req.URL.Path, p.appCfg.Routing.Path, "", 1)
	proxyUrl := p.backendUrl + sanitizedPath

	p.proxy.HandleProxyRequest(rw, req, proxyUrl, p.appName, p.appCfg)
}

func (d *abstractPathBasedDispatcher) buildOptionsHandler(inner httprouter.Handle) httprouter.Handle {
	return func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		recorder := httptest.NewRecorder()

		inner(recorder, req, params)

		allow := recorder.Header().Get("Allow")
		if allow == "" {
			allow = "GET, POST, PUT, DELETE, PATCH, OPTIONS"
		}

		if d.cfg.Proxy.OptionsConfiguration.CORS {
			recorder.Header().Set("Access-Control-Allow-Methods", allow)

			if recorder.Header().Get("Access-Control-Allow-Origin") == "" {
				recorder.Header().Set("Access-Control-Allow-Origin", "*")
			}

			if recorder.Header().Get("Access-Control-Allow-Credentials") == "" {
				recorder.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if recorder.Header().Get("Access-Control-Allow-Headers") == "" {
				recorder.Header().Set("Access-Control-Allow-Headers", "X-Requested-With, Authorization")
			}

			if recorder.Header().Get("Access-Control-Max-Age") == "" {
				recorder.Header().Set("Access-Control-Max-Age", "86400")
			}
		}

		for key, values := range recorder.Header() {
			for _, value := range values {
				rw.Header().Add(key, value)
			}
		}

		rw.Header().Set("Allow", allow)

		_, err := io.Copy(rw, recorder.Body)
		if err != nil {
			d.log.Errorf("error while reading response body: %s", err)
			rw.WriteHeader(500)
			contentLength, _ := rw.Write([]byte(`{"msg":"internal server error"}`))
			rw.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
			rw.Header().Set("Content-Type", "application/json")
			return
		}

		rw.WriteHeader(recorder.Code)
	}
}

func (d *abstractPathBasedDispatcher) Initialize() error {
	for _, behaviour := range d.behaviours {
		switch t := behaviour.(type) {
		case RoutingBehaviour:
			if err := t.AddRoutes(d.mux); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return nil
}
