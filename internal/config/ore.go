package config

import (
	"fmt"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var oreV *viper.Viper

func LoadOre(flags *pflag.FlagSet) (*OreConfig, error) {
	v := viper.New()

	v.SetDefault("log_level", "info")
	v.SetDefault("verbose", false)
	v.SetDefault("context", "")
	v.SetDefault("servers", map[string]any{})

	v.SetEnvPrefix("ORE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(OreConfigDir())
	for _, dir := range xdg.ConfigDirs {
		v.AddConfigPath(dir + "/ore")
	}

	if err := readOrCreateConfig(v, OreConfigFile()); err != nil {
		return nil, err
	}

	oreV = v

	if flags != nil {
		v = viper.New()
		err := v.MergeConfigMap(oreV.AllSettings())
		if err != nil {
			return nil, err
		}
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

func SetContext(name string) error {
	name = strings.ToLower(name)
	oreV.Set("context", name)
	return saveOre()
}

func SetProject(project string) error {
	ctx := oreV.GetString("context")
	if ctx == "" {
		return fmt.Errorf("no server selected")
	}
	oreV.Set("servers."+ctx+".project", project)
	return saveOre()
}

func SaveServer(name string, srv ServerConfig) error {
	name = strings.ToLower(name)
	oreV.Set("servers."+name+".addr", srv.Addr)
	oreV.Set("servers."+name+".token", srv.Token)
	oreV.Set("servers."+name+".project", srv.Project)
	return saveOre()
}

func RemoveServer(name string) error {
	name = strings.ToLower(name)

	all := oreV.AllSettings()
	servers, _ := all["servers"].(map[string]any)
	if servers == nil {
		return fmt.Errorf("server %q not found", name)
	}
	if _, exists := servers[name]; !exists {
		return fmt.Errorf("server %q not found", name)
	}
	delete(servers, name)
	all["servers"] = servers

	if ctx, _ := all["context"].(string); ctx == name {
		all["context"] = ""
	}

	fresh := viper.New()
	fresh.SetConfigType("yaml")
	for k, val := range all {
		fresh.Set(k, val)
	}
	return fresh.WriteConfigAs(oreConfigPath())
}

func ResolveRemote(cfg *OreConfig) (addr, token, project string, ok bool) {
	if cfg.Context == "" {
		return "", "", "", false
	}
	srv, exists := cfg.Servers[cfg.Context]
	if !exists {
		return "", "", "", false
	}
	return srv.Addr, srv.Token, srv.Project, srv.Addr != ""
}

func saveOre() error {
	return oreV.WriteConfigAs(oreConfigPath())
}

func oreConfigPath() string {
	if p := oreV.ConfigFileUsed(); p != "" {
		return p
	}
	return OreConfigFile()
}
