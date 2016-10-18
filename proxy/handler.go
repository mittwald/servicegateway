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
	"github.com/mittwald/servicegateway/config"
	logging "github.com/op/go-logging"
	"net/http"
	"net/url"
	"strings"
	"net"
	"github.com/mittwald/servicegateway/monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

var redirectRequest error = errors.New("redirect")

type ProxyHandler struct {
	Client  *http.Client
	Logger  *logging.Logger
	Config  *config.Configuration

	metrics *monitoring.PromMetrics
}

func NewProxyHandler(logger *logging.Logger, config *config.Configuration, metrics *monitoring.PromMetrics) *ProxyHandler {
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
		Config: config,
		metrics: metrics,
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

func (p *ProxyHandler) UnavailableError(rw http.ResponseWriter, req *http.Request, appName string) {
	p.metrics.Errors.With(prometheus.Labels{"application": appName, "reason": "upstream_unavailable"}).Inc()

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(503)
	rw.Write([]byte("{\"msg\": \"service unavailable\", \"reason\": \"no can do; sorry.\"}"))
}

func (p *ProxyHandler) HandleProxyRequest(rw http.ResponseWriter, req *http.Request, targetUrl string, appName string, appCfg *config.Application) {
	var totalStart, upstreamStart time.Time

	totalStart = time.Now()

	proxyReq, err := http.NewRequest(req.Method, targetUrl, req.Body)
	if err != nil {
		p.UnavailableError(rw, req, appName)
		return
	}

	for header, values := range req.Header {
		for _, value := range values {
			proxyReq.Header.Add(header, value)
		}
	}

	proxyReq.Header.Set("Host", req.Host)

	forwardedFor := req.Header.Get("X-Forwarded-For")
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)

	if forwardedFor != "" {
		proxyReq.Header.Set("X-Forwarded-For", forwardedFor + ", " + ip)
	} else {
		proxyReq.Header.Set("X-Forwarded-For", ip)
	}

	for header, value := range p.Config.Proxy.SetRequestHeaders {
		proxyReq.Header.Set(header, value)
	}

	if appCfg.Backend.Username != "" {
		proxyReq.SetBasicAuth(appCfg.Backend.Username, appCfg.Backend.Password)
	}

	proxyReq.URL.RawQuery = req.URL.RawQuery

	upstreamStart = time.Now()

	proxyRes, err := p.Client.Do(proxyReq)
	if err != nil {
		if uerr, ok := err.(*url.Error); ok == false || uerr.Err != redirectRequest {
			p.Logger.Errorf("could not proxy request to %s: %s", targetUrl, uerr)
			p.UnavailableError(rw, req, appName)
			return
		}
	}

	p.metrics.UpstreamResponseTimes.With(prometheus.Labels{"application": appName}).Observe(time.Now().Sub(upstreamStart).Seconds())

	for header, values := range proxyRes.Header {
		if _, ok := p.Config.Proxy.StripResponseHeaders[header]; ok {
			continue
		}

		for _, value := range values {
			rw.Header().Add(header, value)
		}
	}

	for header, value := range p.Config.Proxy.SetResponseHeaders {
		rw.Header().Set(header, value)
	}

	rw.WriteHeader(proxyRes.StatusCode)

	reader := bufio.NewReader(proxyRes.Body)
	_, err = reader.WriteTo(rw)

	defer proxyRes.Body.Close()
	p.metrics.TotalResponseTimes.With(prometheus.Labels{"application": appName}).Observe(time.Now().Sub(totalStart).Seconds())

	if err != nil {
		p.Logger.Errorf("error while writing response body: %s", err)
	}
}
