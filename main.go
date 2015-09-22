package main

import (
	"flag"
	"mittwald.de/servicegateway/config"
	"encoding/json"
	"io/ioutil"
	"github.com/go-zoo/bone"
	"net/http"
	"mittwald.de/servicegateway/proxy"
	"github.com/garyburd/redigo/redis"
	logging "github.com/op/go-logging"
	"os"
	"mittwald.de/servicegateway/auth"
	"fmt"
	"mittwald.de/servicegateway/dispatcher"
	"mittwald.de/servicegateway/cache"
	"mittwald.de/servicegateway/ratelimit"
)

type StartupConfig struct {
	ConfigDir string
	Port int
	Debug bool
}

func main() {
	startup := StartupConfig{}

	flag.StringVar(&startup.ConfigDir, "config-dir", "/etc/charon", "configuration directory")
	flag.IntVar(&startup.Port, "port", 8080, "HTTP port to listen on")
	flag.BoolVar(&startup.Debug, "debug", false, "enable to add debug information to each request")
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

	redisPool := redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", cfg.Redis)
	}, 8)

	bone := bone.New()

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

	disp, err := dispatcher.NewPathBasedDispatcher(
		&cfg,
		bone,
		logging.MustGetLogger("dispatch"),
		dispatcher.ProxyHandler(handler),
		dispatcher.AuthHandler(authHandler),
		dispatcher.CachingMiddleware(cache),
		dispatcher.RateLimitingMiddleware(rlim),
	)

	if err != nil {
		logger.Fatal("error while creating proxy builder: %s", err)
	}

	for name, appCfg := range cfg.Applications {
		logger.Info("Registering application %s", name)
		if err := disp.RegisterApplication(name, appCfg); err != nil {
			logger.Panic(err)
		}
	}

	listenAddress := fmt.Sprintf(":%d", startup.Port)
	logger.Info("Listening on address %s", listenAddress)

	err = http.ListenAndServe(listenAddress, bone)
	if err != nil {
		logger.Panic(err)
	}
}
