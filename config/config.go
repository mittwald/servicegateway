package config

type Configuration struct {
	Applications map[string]Application
}

type Application struct {
	Routing Routing
	Backend Backend
	Auth Auth
}

type Routing struct {
	Type string
	Path string
	Patterns map[string]string
	Hostname string
}

type Backend struct {
	Url string
}

type Auth struct {
	Disable bool
}
