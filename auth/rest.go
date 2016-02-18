package auth

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
	"github.com/mittwald/servicegateway/config"
	"net/http"
	"github.com/op/go-logging"
	"github.com/julienschmidt/httprouter"
)

type RestAuthDecorator struct {
	authHandler *AuthenticationHandler
	tokenStore TokenStore
	logger *logging.Logger
}

func NewRestAuthDecorator(authHandler *AuthenticationHandler, tokenStore TokenStore, logger *logging.Logger) *RestAuthDecorator {
	return &RestAuthDecorator{
		authHandler: authHandler,
		tokenStore: tokenStore,
		logger: logger,
	}
}

func (a *RestAuthDecorator) DecorateHandler(orig httprouter.Handle, appCfg *config.Application) httprouter.Handle {
	var writer TokenWriter

	switch appCfg.Auth.Writer.Mode {
	case "header":
		writer = &HeaderTokenWriter{HeaderName: appCfg.Auth.Writer.Name}
	case "authorization":
		writer = &AuthorizationTokenWriter{}
	default:
		writer = &HeaderTokenWriter{HeaderName: "X-JWT"}
		a.logger.Error("bad token writer: %s", appCfg.Auth.Writer.Mode)
	}

	return func(res http.ResponseWriter, req *http.Request, p httprouter.Params) {
		authenticated, token, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			a.logger.Error("authentication error: %s", err)

			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(503)
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
		} else if !authenticated {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(403)
			res.Write([]byte("{\"msg\": \"not authenticated\"}"))
		} else {
			writer.WriteTokenToRequest(token, req)
			orig(res, req, p)
		}
	}
}

func (a *RestAuthDecorator) RegisterRoutes(mux *httprouter.Router) error {
	return nil
}
