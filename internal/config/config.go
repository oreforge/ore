package config

type OreConfig struct {
	File     string `mapstructure:"file"`
	LogLevel string `mapstructure:"log_level"`
	Verbose  bool   `mapstructure:"verbose"`
}

type OredConfig struct {
	Addr     string     `mapstructure:"addr"`
	LogLevel string     `mapstructure:"log_level"`
	Auth     AuthConfig `mapstructure:"auth"`
	TLS      TLSConfig  `mapstructure:"tls"`
}

type AuthConfig struct {
	Token string `mapstructure:"token"`
}

type TLSConfig struct {
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}
