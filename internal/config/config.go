package config

import "time"

type OreConfig struct {
	LogLevel string       `mapstructure:"log_level"`
	Verbose  bool         `mapstructure:"verbose"`
	Remote   RemoteConfig `mapstructure:"remote"`
}

type RemoteConfig struct {
	Addr    string     `mapstructure:"addr"`
	Project string     `mapstructure:"project"`
	Auth    AuthConfig `mapstructure:"auth"`
}

type OredConfig struct {
	Addr       string        `mapstructure:"addr"`
	LogLevel   string        `mapstructure:"log_level"`
	Projects   string        `mapstructure:"projects"`
	BindMounts bool          `mapstructure:"bind_mounts"`
	Auth       AuthConfig    `mapstructure:"auth"`
	GitPoll    GitPollConfig `mapstructure:"git_poll"`
}

type GitPollConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Interval time.Duration `mapstructure:"interval"`
	OnUpdate string        `mapstructure:"on_update"`
}

type AuthConfig struct {
	Token string `mapstructure:"token"`
}
