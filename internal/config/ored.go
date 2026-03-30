package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

func LoadOred() (*OredConfig, error) {
	v := viper.New()

	v.SetDefault("addr", ":9090")
	v.SetDefault("log_level", "info")
	v.SetDefault("projects", OredProjectsDir())
	v.SetDefault("bind_mounts", false)
	v.SetDefault("auth.token", "")
	v.SetDefault("git_poll.enabled", false)
	v.SetDefault("git_poll.interval", "5m")
	v.SetDefault("git_poll.on_update", "deploy")

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

		token, genErr := generateToken()
		if genErr != nil {
			return nil, genErr
		}
		v.Set("auth.token", token)

		_ = os.MkdirAll(OredConfigDir(), 0o755)
		if writeErr := v.WriteConfigAs(filepath.Join(OredConfigDir(), "config.yaml")); writeErr != nil {
			return nil, writeErr
		}
	}

	if v.GetString("auth.token") == "" {
		token, err := generateToken()
		if err != nil {
			return nil, err
		}
		v.Set("auth.token", token)
		_ = v.WriteConfig()
	}

	cfg := &OredConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.Projects, 0o755); err != nil {
		return nil, err
	}

	return cfg, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
