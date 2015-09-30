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
	"os"
	"github.com/hashicorp/consul/api"
	"strings"
	"github.com/mailgun/manners"
)

type StartupConfig struct {
	ConfigSource    string
	ConfigFile      string
	DispatchingMode string
	ConsulBaseKey   string
	Port            int
	Debug           bool
}

func main() {
	startup := StartupConfig{}

	flag.StringVar(&startup.ConfigFile, "config", "/etc/servicegateway.json", "configuration file")
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
	data, err := ioutil.ReadFile(startup.ConfigFile)
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

	consulClient, err := api.NewClient(consulConfig)
	if err != nil {
		logger.Panic(err)
	}

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

	listenAddress := fmt.Sprintf(":%d", startup.Port)
	done := make(chan bool)

	go func() {
		var lastIndex uint64 = 0
		var err error

		dispChan := make(chan dispatcher.Dispatcher)
		go func() {
			for disp := range dispChan {
				logger.Info("starting dispatcher on address %s", listenAddress)
				manners.ListenAndServe(listenAddress, disp)
			}
		}()

		for {
			var dispatcher dispatcher.Dispatcher
			dispatcher, lastIndex, err = buildDispatcher(&startup, &cfg, consulClient, handler, cache, authHandler, rlim, logger, lastIndex)
			if err != nil {
				logger.Panic(err)
			}

			manners.Close()
			dispChan <- dispatcher
		}
	}()

	logger.Info("waiting to die")
	<- done
}

func buildDispatcher(
	startup *StartupConfig,
	cfg *config.Configuration,
	consul *api.Client,
	handler *proxy.ProxyHandler,
	cch cache.CacheMiddleware,
	authHandler auth.AuthDecorator,
	rlim ratelimit.RateLimitingMiddleware,
	logger *logging.Logger,
	lastIndex uint64,
) (dispatcher.Dispatcher, uint64, error) {
	var disp dispatcher.Dispatcher
	var err error
	var meta *api.QueryMeta
	var applications api.KVPairs

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
		return nil, 0, fmt.Errorf("error while creating proxy builder: %s", err)
	}

	// Order is important here! Behaviours will be called in LIFO order;
	// behaviours that are added last will be called first!
	disp.AddBehaviour(dispatcher.NewCachingBehaviour(cch))
	disp.AddBehaviour(dispatcher.NewAuthenticationBehaviour(authHandler))
	disp.AddBehaviour(dispatcher.NewRatelimitBehaviour(rlim))

	applicationConfigBase := startup.ConsulBaseKey + "/applications"
	queryOpts := api.QueryOptions{WaitIndex: lastIndex}

	logger.Info("loading application config from KV %s", applicationConfigBase)
	applications, meta, err = consul.KV().List(applicationConfigBase, &queryOpts)
	if err != nil {
		return nil, 0, err
	}

	for _, appJson := range applications {
		var appCfg config.Application

		if err := json.Unmarshal(appJson.Value, &appCfg); err != nil {
			return nil, 0, err
		}

		name := strings.TrimPrefix(appJson.Key, applicationConfigBase + "/")
		logger.Info("registering application '%s' from Consul", name)
		if err := disp.RegisterApplication(name, appCfg); err != nil {
			return nil, 0, err
		}
	}

	for name, appCfg := range cfg.Applications {
		logger.Info("registering application '%s' from local config", name)
		if err := disp.RegisterApplication(name, appCfg); err != nil {
			return nil, 0, err
		}
	}

	if err = disp.Initialize(); err != nil {
		return nil, 0, err
	}

	return disp, meta.LastIndex, nil
}
