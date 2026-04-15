package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	v.SetDefault("token", "")
	v.SetDefault("projects", OredProjectsDir())
	v.SetDefault("backups", OredBackupsDir())
	v.SetDefault("bind_mounts", false)

	v.SetEnvPrefix("ORED")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(OredConfigDir())
	for _, dir := range xdg.ConfigDirs {
		v.AddConfigPath(dir + "/ored")
	}

	if err := readOrCreateConfig(v, OredConfigFile()); err != nil {
		return nil, err
	}

	cfg := &OredConfig{}
	if err := v.Unmarshal(cfg, decodeHook()); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.Projects, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Backups, 0o755); err != nil {
		return nil, err
	}

	return cfg, nil
}

func EnsureToken(cfg *OredConfig) error {
	if cfg.Token != "" {
		return nil
	}
	token, err := generateToken()
	if err != nil {
		return err
	}
	cfg.Token = token

	v := viper.New()
	v.SetConfigFile(OredConfigFile())
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	v.Set("token", token)
	return v.WriteConfig()
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func readOrCreateConfig(v *viper.Viper, path string) error {
	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return err
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := v.SafeWriteConfigAs(path); err != nil {
			if _, ok := errors.AsType[viper.ConfigFileAlreadyExistsError](err); !ok {
				return err
			}
		}
	}
	return nil
}

func decodeHook() viper.DecoderConfigOption {
	return viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))
}
