package auth

import (
	"net/http"
	"mittwald.de/servicegateway/config"
	"errors"
	"fmt"
	"github.com/go-zoo/bone"
	logging "github.com/op/go-logging"
	"github.com/garyburd/redigo/redis"
)

var InvalidCredentialsError error = errors.New("invalid credentials given")

type AuthDecorator interface {
	DecorateHandler(http.Handler, *config.Application) http.Handler
	RegisterRoutes(*bone.Mux) error
}

type AuthenticationRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TimeToLive int `json:"ttl"`
	Providers []string `json:"providers"`
}

func NewAuthDecorator(authConfig *config.GlobalAuth, redisPool *redis.Pool, logger *logging.Logger) (AuthDecorator, error) {
	authHandler, err := NewAuthenticationHandler(authConfig, redisPool, logger)
	if err != nil {
		return nil, err
	}

	switch authConfig.Mode {
	case "graphical":
		return &GraphicalAuthDecorator{
			authHandler,
			authConfig,
			logger,
		}, nil
	case "rest":
		return &RestAuthDecorator{
			authHandler,
		}, nil
	}
	return nil, errors.New(fmt.Sprintf("unsupported authentication mode: '%s'", authConfig.Mode))
}
