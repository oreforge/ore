package config

type OreConfig struct {
	LogLevel string                `mapstructure:"log_level"`
	Verbose  bool                  `mapstructure:"verbose"`
	Context  string                `mapstructure:"context"`
	Nodes    map[string]NodeConfig `mapstructure:"nodes"`
}

type NodeConfig struct {
	Addr    string `mapstructure:"addr"`
	Token   string `mapstructure:"token"`
	Project string `mapstructure:"project"`
}

type OredConfig struct {
	Addr       string `mapstructure:"addr"`
	LogLevel   string `mapstructure:"log_level"`
	Token      string `mapstructure:"token"`
	Projects   string `mapstructure:"projects"`
	Backups    string `mapstructure:"backups"`
	BindMounts bool   `mapstructure:"bind_mounts"`
}
