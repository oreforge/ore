package config

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func LoadOre(flags *pflag.FlagSet) (*OreConfig, error) {
	v := viper.New()

	v.SetDefault("log_level", "info")
	v.SetDefault("verbose", false)
	v.SetDefault("remote.addr", "")
	v.SetDefault("remote.project", "")
	v.SetDefault("remote.auth.token", "")

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
		if err := writeConfig(v, OreConfigDir()); err != nil {
			return nil, err
		}
	}

	syncConfig(v)

	if flags != nil {
		if err := v.BindPFlags(flags); err != nil {
			return nil, err
		}
	}

	cfg := &OreConfig{}
	if err := v.Unmarshal(cfg, decodeHook()); err != nil {
		return nil, err
	}

	return cfg, nil
}

func SaveProject(project string) error {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(OreConfigDir())
	for _, dir := range xdg.ConfigDirs {
		v.AddConfigPath(dir + "/ore")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return err
		}
		if err := writeConfig(v, OreConfigDir()); err != nil {
			return err
		}
	}

	v.Set("remote.project", project)

	configPath := v.ConfigFileUsed()
	if configPath == "" {
		configPath = filepath.Join(OreConfigDir(), "config.yaml")
	}
	return v.WriteConfigAs(configPath)
}
