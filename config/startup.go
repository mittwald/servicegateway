package config

type Startup struct {
	ConfigSource    string
	ConfigFile      string
	DispatchingMode string
	ConsulBaseKey   string
	UiDir           string
	Port            int
	AdminAddress    string
	AdminPort       int
	MonitorAddress  string
	MonitorPort     int
	Debug           bool
	ProfileCpu      string
}

func (s *Startup) IsConsulConfig() bool {
	return len(s.ConsulBaseKey) > 0
}
