package config

type AmqpLoggingConfiguration struct {
	Uri        string `json:"uri"`
	Exchange   string `json:"exchange"`
	UnsafeOnly bool   `json:"unsafe_only"`
}

type ApacheLoggingConfiguration struct {
	Filename string `json:"filename"`
}

type LoggingConfiguration struct {
	Type string `json:"type"`
	AmqpLoggingConfiguration
	ApacheLoggingConfiguration
}
