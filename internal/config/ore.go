package config

import (
	"errors"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func LoadOre(flags *pflag.FlagSet) (*OreConfig, error) {
	v := viper.New()

	v.SetDefault("file", "ore.yaml")
	v.SetDefault("log_level", "info")
	v.SetDefault("verbose", false)

	v.SetEnvPrefix("ORE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(OreConfigDir())
	for _, dir := range xdg.ConfigDirs {
		v.AddConfigPath(dir + "/ore")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	if flags != nil {
		if err := v.BindPFlags(flags); err != nil {
			return nil, err
		}
	}

	cfg := &OreConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
