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
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/op/go-logging"
)

type Dispatcher interface {
	http.Handler
	RegisterApplication(string, config.Application, *config.Configuration) error
	Initialize() error
	AddBehaviour(...Behavior)
}

type Behavior interface {
	Apply(httprouter.Handle, httprouter.Handle, Dispatcher, string, *config.Application, *config.Configuration) (httprouter.Handle, httprouter.Handle, error)
}

type RoutingBehaviour interface {
	AddRoutes(*httprouter.Router) error
}

// avoid `cfg` is unused (structcheck)
//nolint: structcheck
type abstractDispatcher struct {
	cfg *config.Configuration
	mux *httprouter.Router
	prx *proxy.ProxyHandler
	log *logging.Logger

	behaviors []Behavior
}

func (d *abstractDispatcher) setProxy(p *proxy.ProxyHandler) {
	d.prx = p
}

func (d *abstractDispatcher) AddBehaviour(behaviors ...Behavior) {
	d.behaviors = append(d.behaviors, behaviors...)
}
