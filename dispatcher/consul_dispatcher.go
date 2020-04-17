package dispatcher

import (
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/hashicorp/consul/api"
	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/admin"
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/cache"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/httplogging"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/mittwald/servicegateway/ratelimit"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"net/http"
	"regexp"
	"strings"
)

func BuildConsulDispatcher(
	startup *config.Startup,
	cfg *config.Configuration,
	consul *api.Client,
	handler *proxy.ProxyHandler,
	rpool *redis.Pool,
	logger *logging.Logger,
	tokenStore auth.TokenStore,
	tokenVerifier *auth.JwtVerifier,
	httpLoggers []httplogging.HttpLogger,
) (http.Handler, http.Handler, error) {
	var disp Dispatcher
	var err error
	var configs api.KVPairs
	var localCfg = *cfg
	var appCfgs = make(map[string]config.Application)

	dispLogger := logging.MustGetLogger("dispatch")

	switch startup.DispatchingMode {
	case "path":
		disp, err = buildConsulPathDispatcher(&localCfg, dispLogger, handler)
	default:
		err = fmt.Errorf("unsupported dispatching mode: '%s'", startup.DispatchingMode)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("error while creating proxy builder: %s", err)
	}

	applicationConfigBase := startup.ConsulBaseKey + "/applications"

	logger.Infof("loading gateway config from KV %s", startup.ConsulBaseKey)
	configs, _, err = consul.KV().List(startup.ConsulBaseKey, &api.QueryOptions{})
	if err != nil {
		return nil, nil, err
	}

	for _, cfgKVPair := range configs {
		logger.Debugf("found KV pair with key '%s'", cfgKVPair.Key)

		switch strings.TrimPrefix(startup.ConsulBaseKey+"/", cfgKVPair.Key) {
		case "rate_limiting":
			if err := json.Unmarshal(cfgKVPair.Value, &localCfg.RateLimiting); err != nil {
				return nil, nil, fmt.Errorf("JSON error on consul KV pair '%s': %s", cfgKVPair.Key, err)
			}
		}

		if strings.HasPrefix(cfgKVPair.Key, applicationConfigBase) {
			var appCfg config.Application

			if err := json.Unmarshal(cfgKVPair.Value, &appCfg); err != nil {
				return nil, nil, fmt.Errorf("JSON error on consul KV pair '%s': %s", cfgKVPair.Key, err)
			}

			name := strings.TrimPrefix(cfgKVPair.Key, applicationConfigBase+"/")
			appCfgs[name] = appCfg
		}
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

	for name, appCfg := range appCfgs {
		logger.Infof("registering application '%s' from Consul", name)
		if err := disp.RegisterApplication(name, appCfg, cfg); err != nil {
			return nil, nil, err
		}
	}

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

type consulPathDispatcher struct {
	*abstractPathBasedDispatcher
}

func buildConsulPathDispatcher(
	cfg *config.Configuration,
	log *logging.Logger,
	prx *proxy.ProxyHandler,
) (*consulPathDispatcher, error) {
	dispatcher := &consulPathDispatcher{
		abstractPathBasedDispatcher: &abstractPathBasedDispatcher{
			abstractDispatcher: abstractDispatcher{},
		},
	}
	dispatcher.cfg = cfg
	dispatcher.mux = httprouter.New()
	dispatcher.log = log
	dispatcher.prx = prx
	dispatcher.behaviours = make([]Behaviour, 0, 8)

	return dispatcher, nil
}

func (c *consulPathDispatcher) RegisterApplication(name string, appCfg config.Application, config *config.Configuration) error {
	routes := make(map[string]httprouter.Handle)

	backendUrl := appCfg.Backend.Url
	if backendUrl == "" && appCfg.Backend.Service != "" {
		if appCfg.Backend.Tag != "" {
			backendUrl = fmt.Sprintf("http://%s.%s.service.consul", appCfg.Backend.Tag, appCfg.Backend.Service)
		} else {
			backendUrl = fmt.Sprintf("http://%s.service.consul", appCfg.Backend.Service)
		}
	}

	var rewriter proxy.HostRewriter

	if appCfg.Routing.Type == "path" {
		path := strings.TrimRight(appCfg.Routing.Path, "/")
		mapping := map[string]string{
			"/(?P<path>.*)": path + "/:path",
		}

		rewriter, _ = proxy.NewHostRewriter(backendUrl, mapping, c.log)

		closure := new(PathClosure)
		closure.backendUrl = backendUrl
		closure.appName = name
		closure.appCfg = &appCfg
		closure.proxy = c.prx

		routes[path] = closure.Handle
		routes[path+"/*path"] = closure.Handle
	} else if appCfg.Routing.Type == "pattern" {
		re := regexp.MustCompile(":([a-zA-Z0-9]+)")
		mapping := make(map[string]string)

		for pattern, target := range appCfg.Routing.Patterns {
			targetPattern := "^" + re.ReplaceAllString(target, "(?P<$1>[^/]+?)") + "$"
			mapping[targetPattern] = pattern

			parameters := re.FindAllStringSubmatch(pattern, -1)

			closure := new(PatternClosure)
			closure.targetUrl = backendUrl + target
			closure.parameters = parameters
			closure.appName = name
			closure.appCfg = &appCfg
			closure.proxy = c.prx

			routes[pattern] = closure.Handle
		}

		rewriter, _ = proxy.NewHostRewriter(backendUrl, mapping, c.log)
	}

	for route, handler := range routes {
		handler = rewriter.Decorate(handler)

		safeHandler := handler
		unsafeHandler := handler

		for _, behaviour := range c.behaviours {
			var err error
			safeHandler, unsafeHandler, err = behaviour.Apply(safeHandler, unsafeHandler, c, name, &appCfg, config)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		c.mux.GET(route, safeHandler)
		c.mux.HEAD(route, safeHandler)
		c.mux.POST(route, unsafeHandler)
		c.mux.PUT(route, unsafeHandler)
		c.mux.PATCH(route, unsafeHandler)
		c.mux.DELETE(route, unsafeHandler)

		// Register a dedicated OPTIONS handler if it was enabled.
		// If no OPTIONS handler was enabled, simply proxy OPTIONS request through to the backend servers.
		if c.cfg.Proxy.OptionsConfiguration.Enabled {
			c.mux.OPTIONS(route, c.buildOptionsHandler(safeHandler))
		} else {
			c.mux.OPTIONS(route, safeHandler)
		}
	}

	return nil
}
