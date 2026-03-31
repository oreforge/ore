package config

import "time"

type OreConfig struct {
	LogLevel string                  `mapstructure:"log_level"`
	Verbose  bool                    `mapstructure:"verbose"`
	Context  string                  `mapstructure:"context"`
	Servers  map[string]ServerConfig `mapstructure:"servers"`
}

type ServerConfig struct {
	Addr    string `mapstructure:"addr"`
	Token   string `mapstructure:"token"`
	Project string `mapstructure:"project"`
}

type OredConfig struct {
	Addr       string        `mapstructure:"addr"`
	LogLevel   string        `mapstructure:"log_level"`
	Token      string        `mapstructure:"token"`
	Projects   string        `mapstructure:"projects"`
	BindMounts bool          `mapstructure:"bind_mounts"`
	GitPoll    GitPollConfig `mapstructure:"git_poll"`
}

type GitPollConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Interval time.Duration `mapstructure:"interval"`
	OnUpdate string        `mapstructure:"on_update"`
}
