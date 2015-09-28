package proxy

import (
	"bufio"
	"errors"
	"fmt"
	logging "github.com/op/go-logging"
	"github.com/mittwald/servicegateway/config"
	"net/http"
	"net/url"
	"strings"
)

var redirectRequest error = errors.New("redirect")

type ProxyHandler struct {
	Client *http.Client
	Logger *logging.Logger
}

func NewProxyHandler(logger *logging.Logger) *ProxyHandler {
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
			p.Logger.Error("could not proxy request to %s: %s", targetUrl, uerr)
			p.UnavailableError(rw, req)
			return
		}
	}
	for header, values := range proxyRes.Header {
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
		fmt.Println(err.Error())
	}
}
