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
	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/cache"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/ratelimit"
)

type cachingBehaviour struct {
	cache cache.CacheMiddleware
}

type authBehaviour struct {
	auth auth.AuthDecorator
}

type ratelimitBehaviour struct {
	rlim ratelimit.RateLimitingMiddleware
}

func NewCachingBehaviour(c cache.CacheMiddleware) Behavior {
	return &cachingBehaviour{c}
}

func (c *cachingBehaviour) Apply(safe httprouter.Handle, unsafe httprouter.Handle, d Dispatcher, _ string, app *config.Application, config *config.Configuration) (httprouter.Handle, httprouter.Handle, error) {
	if app.Caching.Enabled {
		safe = c.cache.DecorateHandler(safe)

		if app.Caching.AutoFlush {
			unsafe = c.cache.DecorateUnsafeHandler(unsafe)
		}
	}
	return safe, unsafe, nil
}

func NewAuthenticationBehaviour(a auth.AuthDecorator) Behavior {
	return &authBehaviour{a}
}

func (a *authBehaviour) Apply(safe httprouter.Handle, unsafe httprouter.Handle, d Dispatcher, appName string, app *config.Application, config *config.Configuration) (httprouter.Handle, httprouter.Handle, error) {
	if !app.Auth.Disable {
		safe = a.auth.DecorateHandler(safe, appName, app, config)
		unsafe = a.auth.DecorateHandler(unsafe, appName, app, config)
	}
	return safe, unsafe, nil
}

func (a *authBehaviour) AddRoutes(mux *httprouter.Router) error {
	return a.auth.RegisterRoutes(mux)
}

func NewRatelimitBehaviour(rlim ratelimit.RateLimitingMiddleware) Behavior {
	return &ratelimitBehaviour{rlim}
}

func (r *ratelimitBehaviour) Apply(safe httprouter.Handle, unsafe httprouter.Handle, d Dispatcher, _ string, app *config.Application, config *config.Configuration) (httprouter.Handle, httprouter.Handle, error) {
	if app.RateLimiting {
		safe = r.rlim.DecorateHandler(safe)
		unsafe = r.rlim.DecorateHandler(unsafe)
	}
	return safe, unsafe, nil
}
