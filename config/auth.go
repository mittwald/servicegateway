package config

type AuthWriterConfig struct {
	Mode string `json:"mode"`
	Name string `json:"name"`
}

type ProviderAuthConfig struct {
	URL                 string                 `json:"url"`
	Parameters          map[string]interface{} `json:"parameters"`
	AllowAuthentication bool                   `json:"allow_authentication"`
	AuthenticationURI   string                 `json:"authentication_uri"`
}

type ApplicationAuth struct {
	Disable bool             `json:"disable"`
	Writer  AuthWriterConfig `json:"writer"`
}

type GlobalAuth struct {
	Mode               string             `json:"mode"`
	ProviderConfig     ProviderAuthConfig `json:"provider"`
	VerificationKey    []byte             `json:"verification_key"`
	VerificationKeyURL string             `json:"verification_key_url"`
	KeyCacheTTL        string             `json:"key_cache_ttl"`
}
