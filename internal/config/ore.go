package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func LoadOre(flags *pflag.FlagSet) (*OreConfig, error) {
	v := viper.New()

	v.SetDefault("file", "ore.yaml")
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
		_ = os.MkdirAll(OreConfigDir(), 0o755)
		_ = v.SafeWriteConfig()
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

func SaveProject(project string) error {
	configPath := filepath.Join(OreConfigDir(), "config.yaml")

	existing := make(map[string]any)
	data, err := os.ReadFile(configPath)
	if err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	remote, ok := existing["remote"].(map[string]any)
	if !ok {
		remote = make(map[string]any)
	}
	remote["project"] = project
	existing["remote"] = remote

	out, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}

	if mkErr := os.MkdirAll(OreConfigDir(), 0o755); mkErr != nil {
		return mkErr
	}
	return os.WriteFile(configPath, out, 0o600)
}