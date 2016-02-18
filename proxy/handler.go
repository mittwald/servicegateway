package proxy

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
	"bufio"
	"errors"
	"fmt"
	"github.com/mittwald/servicegateway/config"
	logging "github.com/op/go-logging"
	"net/http"
	"net/url"
	"strings"
)

var redirectRequest error = errors.New("redirect")

type ProxyHandler struct {
	Client *http.Client
	Logger *logging.Logger
	Options *config.Configuration
}

func NewProxyHandler(logger *logging.Logger, options *config.Configuration) *ProxyHandler {
	transport := &http.Transport{}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return redirectRequest
		},
	}

	return &ProxyHandler{
		Client: client,
		Logger: logger,
		Options: options,
	}
}

func (p *ProxyHandler) replaceBackendUri(value string, req *http.Request, appCfg *config.Application) string {
	proto := "http"
	if req.TLS != nil {
		proto = "https"
	}

	publicUrl := proto + "://" + req.Host

	if appCfg.Routing.Type == "path" {
		publicUrl = publicUrl + appCfg.Routing.Path
	}

	return strings.Replace(value, appCfg.Backend.Url, publicUrl, -1)
}

func (p *ProxyHandler) UnavailableError(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(503)
	rw.Write([]byte("{\"msg\": \"service unavailable\", \"reason\": \"no can do; sorry.\"}"))
}

func (p *ProxyHandler) HandleProxyRequest(rw http.ResponseWriter, req *http.Request, targetUrl string, appName string, appCfg *config.Application) {
	proxyReq, err := http.NewRequest(req.Method, targetUrl, req.Body)
	if err != nil {
		p.UnavailableError(rw, req)
		return
	}

	proxyReq.Header.Set("Host", req.Host)
	proxyReq.Header.Set("X-Forwarded-For", req.RemoteAddr)

	for header, values := range req.Header {
		for _, value := range values {
			proxyReq.Header.Add(header, value)
		}
	}

	proxyRes, err := p.Client.Do(proxyReq)
	if err != nil {
		if uerr, ok := err.(*url.Error); ok == false || uerr.Err != redirectRequest {
			p.Logger.Error(fmt.Sprintf("could not proxy request to %s: %s", targetUrl, uerr))
			p.UnavailableError(rw, req)
			return
		}
	}
	for header, values := range proxyRes.Header {
		if _, ok := p.Options.Proxy.StripHeaders[header]; ok {
			continue
		}

		if _, ok := p.Options.Http.SetHeaders[header]; ok {
			continue
		}

		for _, value := range values {
			if header == "Location" {
				value = p.replaceBackendUri(value, req, appCfg)
			}
			rw.Header().Add(header, value)
		}
	}

	rw.WriteHeader(proxyRes.StatusCode)

	reader := bufio.NewReader(proxyRes.Body)
	_, err = reader.WriteTo(rw)

	defer proxyRes.Body.Close()

	if err != nil {
		p.Logger.Error("error while writing response body: %s", err)
	}
}
