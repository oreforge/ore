package config

import (
	"errors"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

func LoadOred() (*OredConfig, error) {
	v := viper.New()

	v.SetDefault("addr", ":8080")
	v.SetDefault("log_level", "info")

	v.SetEnvPrefix("ORED")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(OredConfigDir())
	for _, dir := range xdg.ConfigDirs {
		v.AddConfigPath(dir + "/ored")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	cfg := &OredConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
