package config

type OreConfig struct {
	File     string       `mapstructure:"file"`
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
	Addr        string     `mapstructure:"addr"`
	LogLevel    string     `mapstructure:"log_level"`
	ProjectsDir string     `mapstructure:"projects_dir"`
	Auth        AuthConfig `mapstructure:"auth"`
}

type AuthConfig struct {
	Token string `mapstructure:"token"`
}
