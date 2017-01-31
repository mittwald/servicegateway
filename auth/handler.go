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
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
	"github.com/dgrijalva/jwt-go"
	"github.com/robertkrimen/otto"
)

type AuthenticationHandler struct {
	config              *config.GlobalAuth
	storage             TokenStore
	tokenReader         TokenReader
	httpClient          *http.Client
	logger              *logging.Logger
	verifier            *JwtVerifier
	paramMapFunc        *otto.Script

	expCache            map[string]int64
	expLock             sync.RWMutex

	jsVM *otto.Otto
}

func NewAuthenticationHandler(
	cfg *config.GlobalAuth,
	redisPool *redis.Pool,
	tokenStore TokenStore,
	verifier *JwtVerifier,
	logger *logging.Logger,
) (*AuthenticationHandler, error) {
	handler := AuthenticationHandler{
		config:      cfg,
		storage:     tokenStore,
		tokenReader: &BearerTokenReader{store: tokenStore},
		httpClient:  &http.Client{},
		logger:      logger,
		verifier:    verifier,
		expCache:    make(map[string]int64),
		expLock:     sync.RWMutex{},
	}

	if cfg.ProviderConfig.PreAuthenticationHook != "" {
		handler.jsVM = otto.New()
		handler.jsVM.Set("log", func(call otto.FunctionCall) otto.Value {
			format := call.Argument(0).String();
			args := call.ArgumentList[1:]
			values := make([]interface{}, len(args))

			for i := range args {
				values[i], _ = args[i].Export()
			}

			logger.Debugf(format, values...)
			return otto.UndefinedValue()
		})

		script, err := handler.jsVM.Compile(cfg.ProviderConfig.PreAuthenticationHook, nil)
		if err != nil {
			return nil, fmt.Errorf("could not parse JS hook %s: %s", cfg.ProviderConfig.PreAuthenticationHook, err.Error())
		}
		handler.paramMapFunc = script
	}

	return &handler, nil
}

func (h *AuthenticationHandler) Authenticate(username string, password string) (string, error) {
	authRequest := h.config.ProviderConfig.Parameters
	authRequest["username"] = username
	authRequest["password"] = password

	requestURL := h.config.ProviderConfig.Url+"/authenticate"

	if h.paramMapFunc != nil {
		_, err := h.jsVM.Run(h.paramMapFunc)
		if err != nil {
			return "", err
		}

		export, _ := h.jsVM.Get("exports")
		if !export.IsFunction() {
			return "", fmt.Errorf("hook script must export a function!")
		}

		hookResult, err := export.Call(otto.UndefinedValue(), username, password)
		if err != nil {
			return "", fmt.Errorf("error while calling hook function: %s", err.Error())
		}

		hookResultBool, _ := hookResult.ToBoolean()
		if !hookResultBool {
			return "", InvalidCredentialsError
		}

		if !hookResult.IsObject() {
			return "", fmt.Errorf("hook function must return object. is: %s", hookResult.Class())
		}

		body, err := hookResult.Object().Get("body")
		exportedAuthRequest, _ := body.Export()
		newAuthRequest, ok := exportedAuthRequest.(map[string]interface{})

		if ok {
			for k := range newAuthRequest {
				if ottoValue, ok := newAuthRequest[k].(otto.Value); ok {
					newAuthRequest[k], _ = ottoValue.Export()
				}
			}

			authRequest = newAuthRequest
			h.logger.Debugf("hook mapped authentication request to: %s", authRequest)
		}


		url, err := hookResult.Object().Get("url")
		if url.IsString() {
			requestURL = url.String()
			h.logger.Debugf("hook set request URL to: %s", url)
		}
	}

	jsonString, err := json.Marshal(authRequest)
	if err != nil {
		return "", err
	}

	redactedAuthRequest := authRequest
	if _, ok := redactedAuthRequest["password"]; ok {
		redactedAuthRequest["password"] = "*REDACTED*"
	}

	debugJsonString, _ := json.Marshal(redactedAuthRequest)

	h.logger.Infof("authenticating user %s", username)
	h.logger.Debugf("authentication request: %s", debugJsonString)

	req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(jsonString))
	req.Header.Set("Accept", "application/jwt")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusForbidden {
			h.logger.Warningf("invalid credentials for user %s: %s", username, body)
			return "", InvalidCredentialsError
		} else {
			err := fmt.Errorf("unexpected status code %d for user %s: %s", resp.StatusCode, username, body)
			h.logger.Error(err.Error())
			return "", err
		}
	}

	body, _ := ioutil.ReadAll(resp.Body)
	return string(body), nil
}

func (h *AuthenticationHandler) IsAuthenticated(req *http.Request) (bool, string, error) {
	token, err := h.tokenReader.TokenFromRequest(req)
	if err == NoTokenError {
		return false, "", nil
	} else if err != nil {
		h.logger.Warningf("error while reading token from request: %s", err)
		return false, "", err
	}

	h.expLock.RLock()
	exp, ok := h.expCache[token]
	h.expLock.RUnlock()

	if ok && (exp == 0 || exp > time.Now().Unix()) {
		return true, token, nil
	} else if !ok {
		valid, claims, err := h.verifier.VerifyToken(token)
		if err == nil && valid {
			exp, ok := claims["exp"]
			if !ok {
				h.expLock.Lock()
				h.expCache[token] = 0
				h.expLock.Unlock()
				return true, token, nil
			}

			expNum, ok := exp.(float64)
			if !ok {
				return false, "", fmt.Errorf("expiration tstamp is not numeric")
			}

			expNumInt := int64(expNum)
			if expNumInt > time.Now().Unix() {
				h.logger.Debugf("JWT for token %s expires at %d", token, expNumInt)
				h.expLock.Lock()
				h.expCache[token] = expNumInt
				h.expLock.Unlock()

				c := time.After(time.Duration((expNumInt - time.Now().Unix())) * time.Second)
				go func() {
					<- c
					h.expLock.Lock()
					delete(h.expCache, token)
					h.expLock.Unlock()
				}()

				return true, token, nil
			}
		}

		acceptableErrors := jwt.ValidationErrorExpired | jwt.ValidationErrorSignatureInvalid
		if err != nil {
			switch t := err.(type) {
			case *jwt.ValidationError:
				if t.Errors&acceptableErrors != 0 {
					return false, "", nil
				}
			}
			return false, "", err
		}
	}
	return false, "", nil
}
