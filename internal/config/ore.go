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
	v.SetDefault("nodes", map[string]any{})

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
		return fmt.Errorf("no node selected")
	}
	oreV.Set("nodes."+ctx+".project", project)
	return saveOre()
}

func SaveNode(name string, node NodeConfig) error {
	name = strings.ToLower(name)
	oreV.Set("nodes."+name+".addr", node.Addr)
	oreV.Set("nodes."+name+".token", node.Token)
	oreV.Set("nodes."+name+".project", node.Project)
	return saveOre()
}

func RemoveNode(name string) error {
	name = strings.ToLower(name)

	nodes := oreV.GetStringMap("nodes")
	if _, exists := nodes[name]; !exists {
		return fmt.Errorf("node %q not found", name)
	}
	delete(nodes, name)
	oreV.Set("nodes", nodes)

	if oreV.GetString("context") == name {
		oreV.Set("context", "")
	}
	return saveOre()
}

func ResolveRemote(cfg *OreConfig) (addr, token, project string, ok bool) {
	if cfg.Context == "" {
		return "", "", "", false
	}
	node, exists := cfg.Nodes[cfg.Context]
	if !exists {
		return "", "", "", false
	}
	return node.Addr, node.Token, node.Project, node.Addr != ""
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
