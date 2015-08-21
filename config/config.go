package config

type Configuration struct {
	Applications map[string]Application `json:"applications"`
	RateLimiting RateLimiting `json:"rate_limiting"`
}

type Application struct {
	Routing Routing `json:"routing"`
	Backend Backend `json:"backend"`
	Auth Auth `json:"auth"`
	Caching Caching `json:"caching"`
}

type Routing struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Patterns map[string]string `json:"patterns"`
	Hostname string `json:"hostname"`
}

type Backend struct {
	Url string `json:"url"`
}

type Auth struct {
	Disable bool `json:"disable"`
}

type Caching struct {
	Enabled bool `json:"enabled"`
	Ttl int `json:"ttl"`
	AutoFlush bool `json:"auto_flush"`
}

type RateLimiting struct {
	Burst int `json:"burst"`
	RequestsPerSecond int `json:"requests_per_second"`
}
