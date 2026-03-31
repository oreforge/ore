package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/go-viper/mapstructure/v2"
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

		if err := writeConfig(v, OredConfigDir()); err != nil {
			return nil, err
		}
	}

	if v.GetString("auth.token") == "" {
		token, err := generateToken()
		if err != nil {
			return nil, err
		}
		v.Set("auth.token", token)
	}

	syncConfig(v)

	cfg := &OredConfig{}
	if err := v.Unmarshal(cfg, decodeHook()); err != nil {
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

func writeConfig(v *viper.Viper, dir string) error {
	_ = os.MkdirAll(dir, 0o755)
	return v.WriteConfigAs(filepath.Join(dir, "config.yaml"))
}

func syncConfig(v *viper.Viper) {
	if err := v.WriteConfig(); err != nil {
		slog.Warn("failed to sync config to disk", "error", err)
	}
}

func decodeHook() viper.DecoderConfigOption {
	return viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))
}
