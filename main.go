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
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/config"
	"github.com/mittwald/servicegateway/dispatcher"
	"github.com/mittwald/servicegateway/httplogging"
	"github.com/mittwald/servicegateway/monitoring"
	"github.com/mittwald/servicegateway/proxy"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
)

func main() {
	startup := config.Startup{}

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
		err = pprof.StartCPUProfile(f)
		if err != nil {
			logger.Fatal(err)
		}
		defer pprof.StopCPUProfile()

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			for range c {
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

	var monitoringController monitoring.Controller
	monitoringLogger := logging.MustGetLogger("monitoring")

	if startup.IsConsulConfig() {
		consulClient, consulClientErr := cfg.Consul.BuildConsulClient()
		if consulClientErr != nil {
			logger.Panic(err)
		}
		monitoringController, err = monitoring.NewConsulIntegrationMonitoringController(
			startup.MonitorAddress,
			startup.MonitorPort,
			consulClient,
			monitoringLogger,
		)
	} else {
		monitoringController, err = monitoring.NewNoIntegrationMonitoringController(
			startup.MonitorAddress,
			startup.MonitorPort,
			monitoringLogger,
		)
	}

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
			monitoringController.SendShutdown()
			serverShutdown <- true
		}
	}()

	go func() {
		monitoringController.WaitForShutdown()
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

		if startup.IsConsulConfig() {
			var consulClient *api.Client
			consulClient, err = cfg.Consul.BuildConsulClient()
			if err != nil {
				logger.Error(err.Error())
				return
			}

			disp, adminHandler, err = dispatcher.BuildConsulDispatcher(
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
		} else {
			disp, adminHandler, err = dispatcher.BuildNoConsulDispatcher(
				&startup,
				&cfg,
				handler,
				redisPool,
				logger,
				tokenStore,
				tokenVerifier,
				httpLoggers,
			)
		}

		if err != nil {
			logger.Error(err.Error())
			return
		}

		shutdownServers()

		proxyServer = manners.NewWithServer(&http.Server{Addr: listenAddress, Handler: disp})
		adminServer = manners.NewWithServer(&http.Server{Addr: adminListenAddress, Handler: adminHandler})

		logger.Debug("Starting new servers")

		go func() {
			logger.Infof("starting dispatcher on address %s", listenAddress)
			_ = proxyServer.ListenAndServe()
		}()

		go func() {
			logger.Infof("starting admin server on address %s", adminListenAddress)
			_ = adminServer.ListenAndServe()
		}()

	}()

	logger.Info("waiting to die")
	<-done
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
