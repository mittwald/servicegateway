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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
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

func (a *RestAuthDecorator) DecorateHandler(orig httprouter.Handle, appName string, appCfg *config.Application, cfg *config.Configuration) httprouter.Handle {
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

		responseRecorder := httptest.NewRecorder()

		handleError := func(err error, rw http.ResponseWriter, statusCode int) {
			a.logger.Errorf("error while handling authentication request: %s", err)
			rw.Header().Set("Content-Type", "application/json;charset=utf8")
			rw.WriteHeader(statusCode)
			_, _ = rw.Write([]byte(`{"msg":"internal server error"}`))
		}

		authenticated, token, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			handleError(err, res, 503)
			return
		}

		if cfg.Authentication.ProviderConfig.Service == appName ||
			(cfg.Applications[appName].Backend.Url != "" && cfg.Authentication.ProviderConfig.Url != "" &&
				cfg.Applications[appName].Backend.Url == cfg.Authentication.ProviderConfig.Url) {
			goto valid
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
		if token != nil {
			_ = writer.WriteTokenToRequest(token.JWT, req)

			for i := range a.listeners {
				a.listeners[i].OnAuthenticatedRequest(req, token.JWT)
			}
		}

		orig(responseRecorder, req, p)

		// if app was a provider app allow token rewrites
		if cfg.Authentication.ProviderConfig.Service == appName ||
			(cfg.Applications[appName].Backend.Url != "" && cfg.Authentication.ProviderConfig.Url != "" &&
				cfg.Applications[appName].Backend.Url == cfg.Authentication.ProviderConfig.Url) {
			err := rewriteAccessTokens(responseRecorder, req, a)

			if err != nil {
				handleError(err, responseRecorder, 500)
				return
			}
		}

		for headerName, values := range responseRecorder.Header() {
			for _, v := range values {
				res.Header().Add(headerName, v)
			}
		}

		res.WriteHeader(responseRecorder.Code)

		_, _ = io.Copy(res, responseRecorder.Body)

		return

	invalid:
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(403)
		_, _ = res.Write([]byte("{\"msg\": \"not authenticated\"}"))
	}
}

func (a *RestAuthDecorator) RegisterRoutes(mux *httprouter.Router) error {
	if !a.authHandler.config.ProviderConfig.AllowAuthentication {
		return nil
	}

	uri := a.authHandler.config.ProviderConfig.AuthenticationUri
	if uri == "" {
		uri = "/authenticate"
	}

	handleError := func(err error, rw http.ResponseWriter) {
		a.logger.Errorf("error while handling authentication request: %s", err)
		rw.Header().Set("Content-Type", "application/json;charset=utf8")
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte(`{"msg":"internal server error"}`))
	}

	handleIncompleteAuthentication := func(authenticationIncompleteErr *AuthenticationIncompleteError, rw http.ResponseWriter) error {
		rw.Header().Set("Content-Type", "application/json;charset=utf8")
		rw.WriteHeader(202)

		jsonString, err := json.Marshal(authenticationIncompleteErr.AdditionalProperties)
		if err != nil {
			return err
		}
		_, _ = rw.Write(jsonString)
		return nil
	}

	if a.authHandler.config.EnableCORS {
		mux.OPTIONS(
			uri, func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
				setCORSHeaders(rw.Header())
				rw.WriteHeader(200)
			},
		)
	}

	mux.POST(
		uri, func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
			var authRequest ExternalAuthenticationRequest
			var genericBody map[string]interface{}

			h := rw.Header()
			if a.authHandler.config.EnableCORS {
				setCORSHeaders(h)
			}

			requestBody, err := io.ReadAll(req.Body)
			if err != nil {
				handleError(err, rw)
				return
			}

			if err := json.Unmarshal(requestBody, &authRequest); err != nil {
				handleError(err, rw)
				return
			}
			if err := json.Unmarshal(requestBody, &genericBody); err != nil {
				handleError(err, rw)
				return
			}

			authResponse, err := a.authHandler.Authenticate(authRequest.Username, authRequest.Password, genericBody)
			if err == InvalidCredentialsError {
				rw.Header().Set("Content-Type", "application/json;charset=utf8")
				rw.WriteHeader(403)
				_, _ = rw.Write([]byte(`{"msg":"invalid credentials"}`))
				return
			} else if errors.Is(err, AuthenticationIncompleteError{}) {
				if innerErr := handleIncompleteAuthentication(err.(*AuthenticationIncompleteError), rw); innerErr != nil {
					handleError(innerErr, rw)
					return
				}
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
				Token:   token,
				Expires: time.Unix(exp, 0).Format(time.RFC3339),
			}
			jsonResponse, err := json.Marshal(&response)
			if err != nil {
				handleError(err, rw)
				return
			}

			h.Set("Content-Type", "application/json;charset=utf8")
			_, _ = rw.Write(jsonResponse)
		},
	)

	return nil
}

func setCORSHeaders(headers http.Header) {
	headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "X-Requested-With, Authorization, Content-Type")
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Credentials", "true")
}

func rewriteAccessTokens(resp *httptest.ResponseRecorder, req *http.Request, a *RestAuthDecorator) error {
	err := rewriteBodyAccessTokens(resp, req, a)
	if err != nil {
		return err
	}

	err = rewriteHeaderAccessTokens(resp, req, a)
	if err != nil {
		return err
	}

	return rewriteCookieAccessTokens(resp, req, a)
}

func rewriteBodyAccessTokens(resp *httptest.ResponseRecorder, req *http.Request, a *RestAuthDecorator) error {
	if resp.Header().Get("Content-Type") == "application/jwt" {
		jwtBlob, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		jwtResponse := JWTResponse{}
		jwtResponse.JWT = string(jwtBlob)

		token, _, err := a.tokenStore.AddToken(&jwtResponse)
		if err != nil {
			return err
		}

		contentLength, err := resp.WriteString(token)
		if err != nil {
			return err
		}

		resp.Header().Set("Content-Type", "text/plain")
		resp.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))

		return nil
	}

	// rewrite body tokens
	bodyTokenKey := resp.Header().Get("X-Gateway-BodyToken")
	if bodyTokenKey != "" {
		var response map[string]interface{}
		jsonBlob, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		err = json.Unmarshal(jsonBlob, &response)

		if err != nil {
			return err
		}

		bodyToken, ok := response[bodyTokenKey]

		if !ok {
			return err
		}

		jwtResponse := JWTResponse{}
		jwtResponse.JWT = bodyToken.(string)

		token, _, err := a.tokenStore.AddToken(&jwtResponse)
		if err != nil {
			return err
		}

		response[bodyTokenKey] = token
		jsonResponse, err := json.Marshal(&response)
		if err != nil {
			return err
		}
		contentLength, err := resp.Write(jsonResponse)
		if err != nil {
			return err
		}
		resp.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
	}

	return nil
}

func rewriteHeaderAccessTokens(resp *httptest.ResponseRecorder, req *http.Request, a *RestAuthDecorator) error {
	headerTokenKey := resp.Header().Get("X-Gateway-HeaderToken")
	if headerTokenKey != "" {
		header := resp.Header().Get(headerTokenKey)

		jwtResponse := JWTResponse{}
		jwtResponse.JWT = string(header)

		token, _, err := a.tokenStore.AddToken(&jwtResponse)
		if err != nil {
			return err
		}

		resp.Header().Set(headerTokenKey, token)
	}

	return nil
}

func rewriteCookieAccessTokens(resp *httptest.ResponseRecorder, req *http.Request, a *RestAuthDecorator) error {
	cookieTokenKey := resp.Header().Get("X-Gateway-CookieToken")
	if cookieTokenKey != "" {
		cookie := parseCookie(resp, cookieTokenKey)

		if cookie == nil {
			return fmt.Errorf("cookie %s not found", cookieTokenKey)
		}

		jwtResponse := JWTResponse{}
		jwtResponse.JWT = string(cookie.Value)

		token, _, err := a.tokenStore.AddToken(&jwtResponse)
		if err != nil {
			return err
		}

		cookie.Value = token
		http.SetCookie(resp, cookie)
	}

	return nil
}

func parseCookies(resp *httptest.ResponseRecorder) []*http.Cookie {
	return (&http.Response{Header: resp.Header()}).Cookies()
}

func parseCookie(resp *httptest.ResponseRecorder, cookieName string) *http.Cookie {
	cookies := parseCookies(resp)

	for _, cookie := range cookies {
		if cookie.Name == cookieName {
			return cookie
		}
	}

	return nil
}
