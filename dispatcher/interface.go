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
	"github.com/go-zoo/bone"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/op/go-logging"
	"net/http"
)

type Dispatcher interface {
	http.Handler
	RegisterApplication(string, config.Application) error
	Initialize() error
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
