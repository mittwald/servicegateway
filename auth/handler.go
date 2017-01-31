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
	"github.com/dgrijalva/jwt-go"
	"github.com/garyburd/redigo/redis"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type AuthenticationHandler struct {
	config      *config.GlobalAuth
	storage     TokenStore
	tokenReader TokenReader
	httpClient  *http.Client
	logger      *logging.Logger
	verifier    *JWTVerifier

	expCache map[string]int64
	expLock  sync.RWMutex
}

func NewAuthenticationHandler(
	cfg *config.GlobalAuth,
	redisPool *redis.Pool,
	tokenStore TokenStore,
	verifier *JWTVerifier,
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

	return &handler, nil
}

func (h *AuthenticationHandler) Authenticate(username string, password string) (string, error) {
	authRequest := h.config.ProviderConfig.Parameters
	authRequest["username"] = username
	authRequest["password"] = password

	jsonString, err := json.Marshal(authRequest)
	if err != nil {
		return "", err
	}

	redactedAuthRequest := authRequest
	redactedAuthRequest["password"] = "*REDACTED*"

	debugJSONString, _ := json.Marshal(redactedAuthRequest)

	h.logger.Infof("authenticating user %s", username)
	h.logger.Debugf("authentication request: %s", debugJSONString)

	req, err := http.NewRequest("POST", h.config.ProviderConfig.Url+"/authenticate", bytes.NewBuffer(jsonString))
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
			return "", ErrInvalidCredentials
		}

		err := fmt.Errorf("unexpected status code %d for user %s: %s", resp.StatusCode, username, body)
		h.logger.Error(err.Error())
		return "", err
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
					<-c
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
