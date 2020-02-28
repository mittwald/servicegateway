package dispatcher

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/mittwald/servicegateway/admin"
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/cache"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/httplogging"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/mittwald/servicegateway/ratelimit"
	"github.com/op/go-logging"
	"net/http"
)

func BuildNoConsulDispatcher(
	startup *config.Startup,
	cfg *config.Configuration,
	handler *proxy.ProxyHandler,
	rpool *redis.Pool,
	logger *logging.Logger,
	tokenStore auth.TokenStore,
	tokenVerifier *auth.JwtVerifier,
	httpLoggers []httplogging.HttpLogger,
) (http.Handler, http.Handler, error) {
	var disp Dispatcher
	var err error
	var localCfg = *cfg

	dispLogger := logging.MustGetLogger("dispatch")

	switch startup.DispatchingMode {
	case "path":
		disp, err = NewPathBasedDispatcher(&localCfg, dispLogger, handler)
	default:
		err = fmt.Errorf("unsupported dispatching mode: '%s'", startup.DispatchingMode)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("error while creating proxy builder: %s", err)
	}

	authHandler, err := auth.NewAuthenticationHandler(&localCfg.Authentication, rpool, tokenStore, tokenVerifier, logger)
	if err != nil {
		return nil, nil, err
	}

	authDecorator, err := auth.NewAuthDecorator(&localCfg.Authentication, rpool, logging.MustGetLogger("auth"), authHandler, tokenStore, startup.UiDir)
	if err != nil {
		return nil, nil, err
	}

	rlim, err := ratelimit.NewRateLimiter(localCfg.RateLimiting, rpool, logging.MustGetLogger("ratelimiter"))
	if err != nil {
		logger.Fatalf("error while configuring rate limiting: %s", err)
	}

	cch := cache.NewCache(4096)

	// Order is important here! Behaviours will be called in LIFO order;
	// behaviours that are added last will be called first!
	disp.AddBehaviour(NewCachingBehaviour(cch))
	disp.AddBehaviour(NewAuthenticationBehaviour(authDecorator))
	disp.AddBehaviour(NewRatelimitBehaviour(rlim))

	for name, appCfg := range localCfg.Applications {
		logger.Infof("registering application '%s' from local config", name)
		if err := disp.RegisterApplication(name, appCfg, cfg); err != nil {
			return nil, nil, err
		}
	}

	if err = disp.Initialize(); err != nil {
		return nil, nil, err
	}

	adminLogger, err := logging.GetLogger("admin-api")
	if err != nil {
		return nil, nil, err
	}

	adminServer, err := admin.NewAdminServer(tokenStore, tokenVerifier, authHandler, adminLogger)
	if err != nil {
		return nil, nil, err
	}

	var server http.Handler = disp

	for _, httpLogger := range httpLoggers {
		if listener, ok := httpLogger.(auth.AuthRequestListener); ok {
			authDecorator.RegisterRequestListener(listener)
		}

		server, err = httpLogger.Wrap(server)
		if err != nil {
			return nil, nil, err
		}
	}

	return server, adminServer, nil
}
