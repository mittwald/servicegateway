package config

type Configuration struct {
	Applications map[string]Application `json:"applications"`
	RateLimiting RateLimiting `json:"rate_limiting"`
	Authentication GlobalAuth `json:"authentication"`
	Consul ConsulConfiguration `json:"consul"`
	Redis string `json:"redis"`
}

type Application struct {
	Routing Routing `json:"routing"`
	Backend Backend `json:"backend"`
	Auth ApplicationAuth `json:"auth"`
	Caching Caching `json:"caching"`
	RateLimiting bool `json:"rate_limiting"`
}

type Routing struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Patterns map[string]string `json:"patterns"`
	Hostname string `json:"hostname"`
}

type Backend struct {
	Url string `json:"url"`
	Service string `json:"service"`
	Tag string `json:"tag"`
}

type ApplicationAuth struct {
	Disable bool `json:"disable"`
}

type GlobalAuth struct {
	Mode string `json:"mode"`
	StorageConfig StorageAuthConfig `json:"storage"`
	GraphicalConfig GraphicalAuthConfig `json:"graphical"`
	ProviderConfig ProviderAuthConfig `json:"provider"`
	VerificationKey []byte `json:"verification_key"`
	VerificationKeyUrl string `json:"verification_key_url"`
	KeyCacheTtl string `json:"key_cache_ttl"`
}

type ConsulConfiguration struct {
	Host string `json:"host"`
	Port int `json:"port"`
}

type StorageAuthConfig struct {
	Mode string `json:"mode"`
	Name string `json:"name"`
	CookieDomain string `json:"cookie_domain"`
	CookieHttpOnly bool `json:"cookie_httponly"`
	CookieSecure bool `json:"cookie_secure"`
}

type GraphicalAuthConfig struct {
	LoginRoute string `json:"login_route"`
}

type ProviderAuthConfig struct {
	Url string `json:"url"`
	ParameterFormat string `json:"parameter_format"`
	Parameters map[string]interface{} `json:"parameters"`
}

type Caching struct {
	Enabled bool `json:"enabled"`
	Ttl int `json:"ttl"`
	AutoFlush bool `json:"auto_flush"`
}

type RateLimiting struct {
	Burst int `json:"burst"`
	Window string `json:"window"`
	RequestsPerSecond int `json:"requests_per_second"`
}
