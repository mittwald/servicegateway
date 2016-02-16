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
	"errors"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/go-zoo/bone"
	"github.com/mittwald/servicegateway/config"
	logging "github.com/op/go-logging"
	"net/http"
)

var InvalidCredentialsError error = errors.New("invalid credentials given")

type AuthDecorator interface {
	DecorateHandler(http.Handler, *config.Application) http.Handler
	RegisterRoutes(*bone.Mux) error
}

type AuthenticationRequest struct {
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	TimeToLive int      `json:"ttl"`
	Providers  []string `json:"providers"`
}

func NewAuthDecorator(
	authConfig *config.GlobalAuth,
	redisPool *redis.Pool,
	logger *logging.Logger,
	tokenStore TokenStore,
	uiDir string,
) (AuthDecorator, error) {
	authHandler, err := NewAuthenticationHandler(authConfig, redisPool, tokenStore, logger)
	if err != nil {
		return nil, err
	}

	switch authConfig.Mode {
	case "rest":
		return NewRestAuthDecorator(authHandler, tokenStore), nil
	}
	return nil, errors.New(fmt.Sprintf("unsupported authentication mode: '%s'", authConfig.Mode))
}
