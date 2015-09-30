package dispatcher

import (
	"fmt"
	"github.com/go-zoo/bone"
	"github.com/op/go-logging"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/proxy"
	"net/http"
)

type handlerPair struct {
	safe   http.Handler
	unsafe http.Handler
}

type hostBasedDispatcher struct {
	abstractDispatcher
	handlers map[string]handlerPair
}

func NewHostBasedDispatcher(
	cfg *config.Configuration,
	log *logging.Logger,
	prx *proxy.ProxyHandler,
) (Dispatcher, error) {
	disp := new(hostBasedDispatcher)
	disp.cfg = cfg
	disp.log = log
	disp.prx = prx
	disp.mux = bone.New()
	disp.handlers = make(map[string]handlerPair)

	return disp, nil
}

func (d *hostBasedDispatcher) RegisterApplication(name string, app config.Application) error {
	if app.Routing.Type != "host" {
		return fmt.Errorf("unsupported routing type '%s' for application '%s'", app.Routing.Type, name)
	}

	if _, ok := d.handlers[app.Routing.Hostname]; ok {
		return fmt.Errorf("another application is already registered for host '%s'", app.Routing.Hostname)
	}

	backendUrl := app.Backend.Url
	if backendUrl == "" && app.Backend.Service != "" {
		if app.Backend.Tag != "" {
			backendUrl = fmt.Sprintf("http://%s.%s.service.consul", app.Backend.Tag, app.Backend.Service)
		} else {
			backendUrl = fmt.Sprintf("http://%s.service.consul", app.Backend.Service)
		}
	}

	var handler http.Handler = http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		proxyUrl := backendUrl + req.URL.Path
		d.prx.HandleProxyRequest(res, req, proxyUrl, name, &app)
	})

	safeHandler := handler
	unsafeHandler := handler

	for _, behaviour := range d.behaviours {
		var err error
		safeHandler, unsafeHandler, err = behaviour.Apply(safeHandler, unsafeHandler, d, &app)
		if err != nil {
			return err
		}
	}

	d.handlers[app.Routing.Hostname] = handlerPair{
		safeHandler,
		unsafeHandler,
	}

	return nil
}

func (d *hostBasedDispatcher) Initialize() error {
	for _, behaviour := range d.behaviours {
		switch t := behaviour.(type) {
		case RoutingBehaviour:
			if err := t.AddRoutes(d.mux); err != nil {
				return err
			}
		}
	}

	d.mux.HandleFunc("/*", func(res http.ResponseWriter, req *http.Request) {
		handler, ok := d.handlers[req.Host]
		if !ok {
			d.log.Warning("unknown hostname: '%s'", req.Host)
			res.WriteHeader(http.StatusNotFound)
			return
		}

		if req.Method == "GET" || req.Method == "HEAD" || req.Method == "OPTIONS" {
			handler.safe.ServeHTTP(res, req)
		} else {
			handler.unsafe.ServeHTTP(res, req)
		}
	})

	return nil
}

func (d *hostBasedDispatcher) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	d.mux.ServeHTTP(res, req)
}
