package main

import (
	"flag"
	yaml "gopkg.in/yaml.v2"
	"mittwald.de/charon/config"
	"io/ioutil"
	"fmt"
	"github.com/go-zoo/bone"
	"net/http"
	"mittwald.de/charon/proxy"
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

	cfg := config.Configuration{}
	data, err := ioutil.ReadFile("apis.yaml")
	if err != nil {
		panic(err)
	}

	yaml.Unmarshal(data, &cfg)

	fmt.Println(cfg)

	bone := bone.New()

	handler := proxy.NewProxyHandler()
	cache := proxy.NewCache(4096)

	builder := proxy.NewProxyBuilder(handler, cache)

//	e := echo.New()
//	e.SetDebug(true)

	for name, appCfg := range cfg.Applications {
		builder.BuildHandler(bone, name, appCfg)
	}

	http.ListenAndServe(":2000", bone)
//	e.Run(":2000")
}
