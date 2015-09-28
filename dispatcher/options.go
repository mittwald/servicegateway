package dispatcher

import (
	"github.com/mittwald/servicegateway/proxy"
)

type dispatcherSetters interface {
	setProxy(*proxy.ProxyHandler)
}

type DispatcherOption func(dispatcherSetters) error

func ProxyHandler(prx *proxy.ProxyHandler) DispatcherOption {
	return func(d dispatcherSetters) error {
		d.setProxy(prx)
		return nil
	}
}
