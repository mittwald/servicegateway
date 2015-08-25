package main

import (
	"flag"
	"mittwald.de/charon/config"
	"encoding/json"
	"io/ioutil"
	"github.com/go-zoo/bone"
	"net/http"
	"mittwald.de/charon/proxy"
	"github.com/garyburd/redigo/redis"
	logging "github.com/op/go-logging"
	"os"
)

type StartupConfig struct {
	ConfigDir string
	Debug bool
}

func main() {
	startup := StartupConfig{}

	flag.StringVar(&startup.ConfigDir, "config-dir", "/etc/charon", "configuration directory")
	flag.BoolVar(&startup.Debug, "debug", false, "enable to add debug information to each request")
	flag.Parse()

	logger := logging.MustGetLogger("startup")
	format := logging.MustStringFormatter("%{color}%{time:15:04:05.000} %{module} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}")
	backend := logging.NewLogBackend(os.Stderr, "", 0)

	logging.SetBackend(logging.NewBackendFormatter(backend, format))
	logger.Info("Completed startup")

	cfg := config.Configuration{}
	data, err := ioutil.ReadFile("apis.json")
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
		return redis.Dial("tcp", "localhost:6379")
	}, 8)

	bone := bone.New()

	handler := proxy.NewProxyHandler()
	cache := proxy.NewCache(4096)
	throttler := proxy.NewThrottler(cfg.RateLimiting, redisPool, logging.MustGetLogger("ratelimiter"))

	builder := proxy.NewProxyBuilder(handler, cache, throttler)

//	e := echo.New()
//	e.SetDebug(true)

	for name, appCfg := range cfg.Applications {
		builder.BuildHandler(bone, name, appCfg)
	}

	http.ListenAndServe(":2000", bone)
//	e.Run(":2000")
}
