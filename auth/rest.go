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
	"net/http"

	"encoding/json"
	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
	"io/ioutil"
	"time"
)

type RestAuthDecorator struct {
	authHandler *AuthenticationHandler
	tokenStore  TokenStore
	logger      *logging.Logger
	listeners   []AuthRequestListener
}

type ExternalAuthenticationRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ExternalAuthenticationResponse struct {
	Token   string `json:"token"`
	Expires string `json:"expires,omitempty"`
}

func NewRestAuthDecorator(authHandler *AuthenticationHandler, tokenStore TokenStore, logger *logging.Logger) *RestAuthDecorator {
	return &RestAuthDecorator{
		authHandler: authHandler,
		tokenStore:  tokenStore,
		logger:      logger,
		listeners:   make([]AuthRequestListener, 0),
	}
}

func (a *RestAuthDecorator) RegisterRequestListener(listener AuthRequestListener) {
	a.listeners = append(a.listeners, listener)
}

func (a *RestAuthDecorator) DecorateHandler(orig httprouter.Handle, appCfg *config.Application) httprouter.Handle {
	var writer TokenWriter

	switch appCfg.Auth.Writer.Mode {
	case "header":
		writer = &HeaderTokenWriter{HeaderName: appCfg.Auth.Writer.Name}
	case "authorization":
		writer = &AuthorizationTokenWriter{}
	case "":
		writer = &HeaderTokenWriter{HeaderName: "X-JWT"}
	default:
		writer = &HeaderTokenWriter{HeaderName: "X-JWT"}
		a.logger.Errorf("bad token writer: %s", appCfg.Auth.Writer.Mode)
	}

	return func(res http.ResponseWriter, req *http.Request, p httprouter.Params) {
		if req.Method == "OPTIONS" {
			orig(res, req, p)
		}

		authenticated, token, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			a.logger.Errorf("authentication error: %s", err)

			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(503)
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
		} else if !authenticated {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(403)
			res.Write([]byte("{\"msg\": \"not authenticated\"}"))
		} else {
			writer.WriteTokenToRequest(token, req)

			for i := range a.listeners {
				a.listeners[i].OnAuthenticatedRequest(req, token)
			}

			orig(res, req, p)
		}
	}
}

func (a *RestAuthDecorator) RegisterRoutes(mux *httprouter.Router) error {
	if !a.authHandler.config.ProviderConfig.AllowAuthentication {
		return nil
	}

	uri := a.authHandler.config.ProviderConfig.AuthenticationURI
	if uri == "" {
		uri = "/authenciate"
	}

	handleError := func(err error, rw http.ResponseWriter) {
		a.logger.Errorf("error while handling authentication request: %s", err)
		rw.Header().Set("Content-Type", "application/json;charset=utf8")
		rw.WriteHeader(500)
		rw.Write([]byte(`{"msg":"internal server error"}`))
	}

	mux.POST(uri, func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		var authRequest ExternalAuthenticationRequest

		requestBody, err := ioutil.ReadAll(req.Body)
		if err != nil {
			handleError(err, rw)
			return
		}

		if err := json.Unmarshal(requestBody, &authRequest); err != nil {
			handleError(err, rw)
			return
		}

		jwt, err := a.authHandler.Authenticate(authRequest.Username, authRequest.Password)
		if err == ErrInvalidCredentials {
			rw.Header().Set("Content-Type", "application/json;charset=utf8")
			rw.WriteHeader(403)
			rw.Write([]byte(`{"msg":"invalid credentials"}`))
			return
		} else if err != nil {
			handleError(err, rw)
			return
		}

		token, exp, err := a.tokenStore.AddToken(jwt)
		if err != nil {
			handleError(err, rw)
			return
		}

		response := ExternalAuthenticationResponse{
			Token:   token,
			Expires: time.Unix(exp, 0).Format(time.RFC3339),
		}
		jsonResponse, err := json.Marshal(&response)
		if err != nil {
			handleError(err, rw)
			return
		}

		rw.Header().Set("Content-Type", "application/json;charset=utf8")
		rw.Write(jsonResponse)
	})

	return nil
}
