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
