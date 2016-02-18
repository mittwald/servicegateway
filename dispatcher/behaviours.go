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
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/cache"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/ratelimit"
	"github.com/julienschmidt/httprouter"
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

func NewCachingBehaviour(c cache.CacheMiddleware) DispatcherBehaviour {
	return &cachingBehaviour{c}
}

func (c *cachingBehaviour) Apply(safe httprouter.Handle, unsafe httprouter.Handle, d Dispatcher, app *config.Application) (httprouter.Handle, httprouter.Handle, error) {
	if app.Caching.Enabled {
		safe = c.cache.DecorateHandler(safe)

		if app.Caching.AutoFlush {
			unsafe = c.cache.DecorateUnsafeHandler(unsafe)
		}
	}
	return safe, unsafe, nil
}

func NewAuthenticationBehaviour(a auth.AuthDecorator) DispatcherBehaviour {
	return &authBehaviour{a}
}

func (a *authBehaviour) Apply(safe httprouter.Handle, unsafe httprouter.Handle, d Dispatcher, app *config.Application) (httprouter.Handle, httprouter.Handle, error) {
	if !app.Auth.Disable {
		safe = a.auth.DecorateHandler(safe, app)
		unsafe = a.auth.DecorateHandler(unsafe, app)
	}
	return safe, unsafe, nil
}

func (a *authBehaviour) AddRoutes(mux *httprouter.Router) error {
	return a.auth.RegisterRoutes(mux)
}

func NewRatelimitBehaviour(rlim ratelimit.RateLimitingMiddleware) DispatcherBehaviour {
	return &ratelimitBehaviour{rlim}
}

func (r *ratelimitBehaviour) Apply(safe httprouter.Handle, unsafe httprouter.Handle, d Dispatcher, app *config.Application) (httprouter.Handle, httprouter.Handle, error) {
	if app.RateLimiting {
		safe = r.rlim.DecorateHandler(safe)
		unsafe = r.rlim.DecorateHandler(unsafe)
	}
	return safe, unsafe, nil
}
