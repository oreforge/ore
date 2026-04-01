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

	if v.GetString("token") == "" {
		token, err := generateToken()
		if err != nil {
			return nil, err
		}
		v.Set("token", token)
		if err := v.WriteConfig(); err != nil {
			return nil, err
		}
	}

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

func readOrCreateConfig(v *viper.Viper, path string) error {
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return err
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := v.SafeWriteConfigAs(path); err != nil {
			var alreadyExists viper.ConfigFileAlreadyExistsError
			if !errors.As(err, &alreadyExists) {
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
