package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	logging "github.com/op/go-logging"
	"io/ioutil"
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/cache"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/dispatcher"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/mittwald/servicegateway/ratelimit"
	"net/http"
	"os"
	"github.com/hashicorp/consul/api"
	"strings"
)

type StartupConfig struct {
	ConfigSource    string
	ConfigDir       string
	DispatchingMode string
	ConsulBaseKey   string
	Port            int
	Debug           bool
}

func main() {
	startup := StartupConfig{}

	flag.StringVar(&startup.ConfigDir, "config-dir", "/etc/charon", "configuration directory")
	flag.StringVar(&startup.DispatchingMode, "dispatch", "path", "dispatching mode ('path' or 'host')")
	flag.IntVar(&startup.Port, "port", 8080, "HTTP port to listen on")
	flag.BoolVar(&startup.Debug, "debug", false, "enable to add debug information to each request")
	flag.StringVar(&startup.ConsulBaseKey, "consul-base", "gateway/ui", "base key name for configuration")
	flag.Parse()

	logger := logging.MustGetLogger("startup")
	format := logging.MustStringFormatter("%{color}%{time:15:04:05.000} %{module:12s} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
	backend := logging.NewLogBackend(os.Stderr, "", 0)

	logging.SetBackend(logging.NewBackendFormatter(backend, format))
	logger.Info("Completed startup")

	cfg := config.Configuration{}
	data, err := ioutil.ReadFile(startup.ConfigDir + "/apis.json")
	if err != nil {
		logger.Fatal(err)
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		logger.Fatal(err)
		panic(err)
	}

	logger.Debug("%s", cfg)

	consulConfig := api.DefaultConfig()
	consulConfig.Address = "consul.service.consul:8500"
	consulConfig.Datacenter = "dev"

	redisPool := redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", cfg.Redis)
	}, 8)

	handler := proxy.NewProxyHandler(logging.MustGetLogger("proxy"))
	cache := cache.NewCache(4096)

	rlim, err := ratelimit.NewRateLimiter(cfg.RateLimiting, redisPool, logging.MustGetLogger("ratelimiter"))
	if err != nil {
		logger.Fatal("error while configuring rate limiting: %s", err)
	}

	authHandler, err := auth.NewAuthDecorator(&cfg.Authentication, redisPool, logging.MustGetLogger("auth"))
	if err != nil {
		logger.Fatal("error while configuring authentication: %s", err)
	}

	disp, err := buildDispatcher(&startup, &cfg, consulConfig, handler, cache, authHandler, rlim, logger)
	if err != nil {
		logger.Panic(err)
	}

	listenAddress := fmt.Sprintf(":%d", startup.Port)
	logger.Info("Listening on address %s", listenAddress)

	err = http.ListenAndServe(listenAddress, disp)
	if err != nil {
		logger.Panic(err)
	}
}

func buildDispatcher(
	startup *StartupConfig,
	cfg *config.Configuration,
	consulConfig *api.Config,
	handler *proxy.ProxyHandler,
	cch cache.CacheMiddleware,
	authHandler auth.AuthDecorator,
	rlim ratelimit.RateLimitingMiddleware,
	logger *logging.Logger,
) (dispatcher.Dispatcher, error) {
	var disp dispatcher.Dispatcher
	var err error

	dispLogger := logging.MustGetLogger("dispatch")

	switch startup.DispatchingMode {
	case "path":
		disp, err = dispatcher.NewPathBasedDispatcher(cfg, dispLogger, handler)
	case "host":
		disp, err = dispatcher.NewHostBasedDispatcher(cfg, dispLogger, handler)
	default:
		err = fmt.Errorf("unsupported dispatching mode: '%s'", startup.DispatchingMode)
	}

	if err != nil {
		return nil, fmt.Errorf("error while creating proxy builder: %s", err)
	}

	// Order is important here! Behaviours will be called in LIFO order;
	// behaviours that are added last will be called first!
	disp.AddBehaviour(dispatcher.NewCachingBehaviour(cch))
	disp.AddBehaviour(dispatcher.NewAuthenticationBehaviour(authHandler))
	disp.AddBehaviour(dispatcher.NewRatelimitBehaviour(rlim))

	if startup.ConsulBaseKey != "" {
		applicationConfigBase := startup.ConsulBaseKey + "/applications"

		consul, _ := api.NewClient(consulConfig)
		applications, _, err := consul.KV().List(applicationConfigBase, nil)
		if err != nil {
			return nil, err
		}

		for _, appJson := range applications {
			var appCfg config.Application

			if err := json.Unmarshal(appJson.Value, &appCfg); err != nil {
				return nil, err
			}

			name := strings.TrimPrefix(appJson.Key, applicationConfigBase + "/")
			logger.Info("registering application '%s' from Consul", name)
			if err := disp.RegisterApplication(name, appCfg); err != nil {
				return nil, err
			}
		}
	}

	for name, appCfg := range cfg.Applications {
		logger.Info("registering application '%s' from local config", name)
		if err := disp.RegisterApplication(name, appCfg); err != nil {
			return nil, err
		}
	}

	if err = disp.Initialize(); err != nil {
		return nil, err
	}

	return disp, nil
}