package config

type AuthWriterConfig struct {
	Mode string `json:"mode"`
	Name string `json:"name"`
}

type ProviderAuthConfig struct {
	Url                   string                 `json:"url"`
	Parameters            map[string]interface{} `json:"parameters"`
	PreAuthenticationHook string                 `json:"hook_pre_authentication"`
	AllowAuthentication   bool                   `json:"allow_authentication"`
	AuthenticationUri     string                 `json:"authentication_uri"`
}

type ApplicationAuth struct {
	Disable bool             `json:"disable"`
	Writer  AuthWriterConfig `json:"writer"`
}

type GlobalAuth struct {
	Mode               string             `json:"mode"`
	ProviderConfig     ProviderAuthConfig `json:"provider"`
	VerificationKey    []byte             `json:"verification_key"`
	VerificationKeyUrl string             `json:"verification_key_url"`
	KeyCacheTtl        string             `json:"key_cache_ttl"`
	EnableCORS         boolean            `json:"enable_cors"`
}
