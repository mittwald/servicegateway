package main

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
	"flag"
	"fmt"
	"github.com/braintree/manners"
	"github.com/garyburd/redigo/redis"
	"github.com/hashicorp/consul/api"
	"github.com/mittwald/servicegateway/admin"
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/cache"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/dispatcher"
	"github.com/mittwald/servicegateway/httplogging"
	"github.com/mittwald/servicegateway/monitoring"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/mittwald/servicegateway/ratelimit"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
)

type StartupConfig struct {
	ConfigSource    string
	ConfigFile      string
	DispatchingMode string
	ConsulBaseKey   string
	UiDir           string
	Port            int
	AdminAddress    string
	AdminPort       int
	MonitorAddress  string
	MonitorPort     int
	Debug           bool
	ProfileCpu      string
}

func main() {
	startup := StartupConfig{}

	flag.StringVar(&startup.ConfigFile, "config", "/etc/servicegateway.json", "configuration file")
	flag.StringVar(&startup.DispatchingMode, "dispatch", "path", "dispatching mode ('path' or 'host')")
	flag.IntVar(&startup.Port, "port", 8080, "HTTP port to listen on")
	flag.StringVar(&startup.AdminAddress, "admin-addr", "127.0.0.1", "Address to listen on (administration port)")
	flag.IntVar(&startup.AdminPort, "admin-port", 8081, "HTTP port to listen on (administration port)")
	flag.StringVar(&startup.MonitorAddress, "monitor-addr", "0.0.0.0", "Address to listen on (monitoring port)")
	flag.IntVar(&startup.MonitorPort, "monitor-port", 8082, "HTTP port to listen on (monitoring port)")
	flag.BoolVar(&startup.Debug, "debug", false, "enable to add debug information to each request")
	flag.StringVar(&startup.ConsulBaseKey, "consul-base", "gateway/ui", "base key name for configuration")
	flag.StringVar(&startup.UiDir, "ui-dir", "/usr/share/servicegateway", "directory in which UI files can be found")

	flag.StringVar(&startup.ProfileCpu, "cpu-profile", "", "write CPU profile to file")

	flag.Parse()

	logger := logging.MustGetLogger("startup")
	format := logging.MustStringFormatter("%{color}%{time:15:04:05.000} %{module:12s} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
	backend := logging.NewLogBackend(os.Stderr, "", 0)

	if startup.ProfileCpu != "" {
		f, err := os.Create(startup.ProfileCpu)
		if err != nil {
			logger.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			for _ = range c {
				pprof.StopCPUProfile()
				os.Exit(0)
			}
		}()
	}

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

	logger.Debugf("%s", cfg)

	consulConfig := api.DefaultConfig()
	consulConfig.Address = cfg.Consul.Address()
	consulConfig.Datacenter = cfg.Consul.DataCenter

	consulClient, err := api.NewClient(consulConfig)
	if err != nil {
		logger.Panic(err)
	}

	monitoringController, err := monitoring.NewMonitoringController(
		startup.MonitorAddress,
		startup.MonitorPort,
		consulClient,
		logging.MustGetLogger("monitoring"),
	)
	if err != nil {
		logger.Fatal(err)
	}

	err = monitoringController.Start()
	if err != nil {
		logger.Fatal(err)
	}

	metrics := monitoringController.Metrics()
	if err != nil {
		logger.Fatal(err)
	}

	redisPool := &redis.Pool{
		MaxIdle: 8,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", cfg.Redis.Address, cfg.Redis.DialOptions()...)
			if err != nil {
				return nil, err
			}

			return conn, nil
		},
	}

	tokenVerifier, err := auth.NewJwtVerifier(&cfg.Authentication)
	if err != nil {
		logger.Panic(err)
	}

	tokenStore, err := auth.NewTokenStore(redisPool, tokenVerifier, auth.TokenStoreOptions{})
	if err != nil {
		logger.Panic(err)
	}

	httpLoggers, err := buildLoggers(&cfg, tokenVerifier)
	if err != nil {
		logger.Panic(err)
	}

	handler := proxy.NewProxyHandler(logging.MustGetLogger("proxy"), &cfg, metrics)

	listenAddress := fmt.Sprintf(":%d", startup.Port)
	adminListenAddress := fmt.Sprintf("%s:%d", startup.AdminAddress, startup.AdminPort)

	done := make(chan bool)
	serverShutdown := make(chan bool)
	serverShutdownComplete := make(chan bool)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			logger.Notice("received interrupt signal")
			monitoringController.Shutdown <- true
			serverShutdown <- true
		}
	}()

	go func() {
		<-monitoringController.ShutdownComplete
		<-serverShutdownComplete

		logger.Notice("everything has shut down. exiting process.")

		done <- true
	}()

	go func() {
		var err error
		var proxyServer, adminServer *manners.GracefulServer

		shutdownServers := func() {
			if proxyServer != nil {
				logger.Debug("Closing proxy server")
				proxyServer.Close()
			}

			if adminServer != nil {
				logger.Debug("Closing admin server")
				adminServer.Close()
			}
		}

		go func() {
			<-serverShutdown

			logger.Noticef("received server shutdown request. stopping creating new servers")
			shutdownServers()
			serverShutdownComplete <- true
		}()

		var disp http.Handler
		var adminHandler http.Handler

		disp, adminHandler, err = buildDispatcher(
			&startup,
			&cfg,
			consulClient,
			handler,
			redisPool,
			logger,
			tokenStore,
			tokenVerifier,
			httpLoggers,
		)

		if err != nil {
			logger.Error(err.Error())
		} else {

			shutdownServers()

			proxyServer = manners.NewWithServer(&http.Server{Addr: listenAddress, Handler: disp})
			adminServer = manners.NewWithServer(&http.Server{Addr: adminListenAddress, Handler: adminHandler})

			logger.Debug("Starting new servers")

			go func() {
				logger.Infof("starting dispatcher on address %s", listenAddress)
				proxyServer.ListenAndServe()
			}()

			go func() {
				logger.Infof("starting admin server on address %s", adminListenAddress)
				adminServer.ListenAndServe()
			}()

		}
	}()

	logger.Info("waiting to die")
	<-done
}

func buildDispatcher(
	startup *StartupConfig,
	cfg *config.Configuration,
	consul *api.Client,
	handler *proxy.ProxyHandler,
	rpool *redis.Pool,
	logger *logging.Logger,
	tokenStore auth.TokenStore,
	tokenVerifier *auth.JwtVerifier,
	httpLoggers []httplogging.HttpLogger,
) (http.Handler, http.Handler, error) {
	var disp dispatcher.Dispatcher
	var err error
	var configs api.KVPairs
	var localCfg config.Configuration = *cfg
	var appCfgs map[string]config.Application = make(map[string]config.Application)

	dispLogger := logging.MustGetLogger("dispatch")

	switch startup.DispatchingMode {
	case "path":
		disp, err = dispatcher.NewPathBasedDispatcher(&localCfg, dispLogger, handler)
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
	disp.AddBehaviour(dispatcher.NewCachingBehaviour(cch))
	disp.AddBehaviour(dispatcher.NewAuthenticationBehaviour(authDecorator))
	disp.AddBehaviour(dispatcher.NewRatelimitBehaviour(rlim))

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

func buildLoggers(cfg *config.Configuration, tok *auth.JwtVerifier) ([]httplogging.HttpLogger, error) {
	loggers := make([]httplogging.HttpLogger, len(cfg.Logging))
	for i, loggingConfig := range cfg.Logging {
		loggingLogger, err := logging.GetLogger("logger-" + loggingConfig.Type)
		if err != nil {
			return nil, err
		}

		httpLogger, err := httplogging.LoggerFromConfig(&loggingConfig, loggingLogger, tok)
		if err != nil {
			return nil, err
		}
		loggers[i] = httpLogger
	}
	return loggers, nil
}
