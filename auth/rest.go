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

	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
	"io/ioutil"
	"encoding/json"
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
	Token string `json:"token"`
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

func (a *RestAuthDecorator) DecorateHandler(orig httprouter.Handle, appName string, appCfg *config.Application) httprouter.Handle {
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
			return
		}

		authenticated, token, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			a.logger.Errorf("authentication error: %s", err)

			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(503)
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
			return
		}

		if !authenticated {
			goto invalid
		}

		if token.AllowedApplications != nil && len(token.AllowedApplications) > 0 {
			for i := range token.AllowedApplications {
				if token.AllowedApplications[i] == appName {
					goto valid
				}
			}

			a.logger.Warningf("token is not whitelisted for app %s. whitelisted apps: %s", appName, token.AllowedApplications)
			goto invalid
		}

		valid:
			writer.WriteTokenToRequest(token.JWT, req)

			for i, _ := range a.listeners {
				a.listeners[i].OnAuthenticatedRequest(req, token.JWT)
			}

			orig(res, req, p)
			return

		invalid:
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(403)
			res.Write([]byte("{\"msg\": \"not authenticated\"}"))
	}
}

func (a *RestAuthDecorator) RegisterRoutes(mux *httprouter.Router) error {
	if !a.authHandler.config.ProviderConfig.AllowAuthentication {
		return nil
	}

	uri := a.authHandler.config.ProviderConfig.AuthenticationUri
	if uri == "" {
		uri = "/authenciate"
	}

	handleError := func(err error, rw http.ResponseWriter) {
		a.logger.Errorf("error while handling authentication request: %s", err)
		rw.Header().Set("Content-Type", "application/json;charset=utf8")
		rw.WriteHeader(500)
		rw.Write([]byte(`{"msg":"internal server error"}`))
	}

	if a.authHandler.config.EnableCORS {
		mux.OPTIONS(uri, func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
			setCORSHeaders(rw.Header())
			rw.WriteHeader(200)
		})
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

		authResponse, err := a.authHandler.Authenticate(authRequest.Username, authRequest.Password)
		if err == InvalidCredentialsError {
			rw.Header().Set("Content-Type", "application/json;charset=utf8")
			rw.WriteHeader(403)
			rw.Write([]byte(`{"msg":"invalid credentials"}`))
			return
		} else if err != nil || authResponse == nil {
			handleError(err, rw)
			return
		}

		token, exp, err := a.tokenStore.AddToken(authResponse)
		if err != nil {
			handleError(err, rw)
			return
		}

		response := ExternalAuthenticationResponse{
			Token: token,
			Expires: time.Unix(exp, 0).Format(time.RFC3339),
		}
		jsonResponse, err := json.Marshal(&response)
		if err != nil {
			handleError(err, rw)
			return
		}

		h := rw.Header()

		if a.authHandler.config.EnableCORS {
			setCORSHeaders(h)
		}

		h.Set("Content-Type", "application/json;charset=utf8")
		rw.Write(jsonResponse)
	})

	return nil
}

func setCORSHeaders(headers http.Header) {
	headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "X-Requested-With, Authorization, Content-Type")
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Credentials", "true")
}
