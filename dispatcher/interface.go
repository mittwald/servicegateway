package dispatcher

import (
	"github.com/go-zoo/bone"
	"github.com/op/go-logging"
	"mittwald.de/servicegateway/config"
	"mittwald.de/servicegateway/proxy"
	"net/http"
)

type Dispatcher interface {
	http.Handler
	RegisterApplication(string, config.Application) error
	AddBehaviour(...DispatcherBehaviour)
}

type DispatcherBehaviour interface {
	Apply(http.Handler, http.Handler, Dispatcher, *config.Application) (http.Handler, http.Handler, error)
}

type RoutingBehaviour interface {
	AddRoutes(*bone.Mux) error
}

type abstractDispatcher struct {
	cfg *config.Configuration
	mux *bone.Mux
	prx *proxy.ProxyHandler
	log *logging.Logger

	behaviours []DispatcherBehaviour
}

func (d *abstractDispatcher) setProxy(p *proxy.ProxyHandler) {
	d.prx = p
}

func (d *abstractDispatcher) AddBehaviour(behaviours ...DispatcherBehaviour) {
	for _, behaviour := range behaviours {
		d.behaviours = append(d.behaviours, behaviour)
	}
}
