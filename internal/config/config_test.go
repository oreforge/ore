package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupXDG(t *testing.T) (configHome, dataHome string) {
	t.Helper()
	configHome = filepath.Join(t.TempDir(), "config")
	dataHome = filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_DIRS", filepath.Join(t.TempDir(), "missing"))
	xdg.Reload()
	t.Cleanup(xdg.Reload)
	return configHome, dataHome
}

func TestResolveRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         OreConfig
		wantAddr    string
		wantToken   string
		wantProject string
		wantOK      bool
	}{
		{
			name:   "no_context",
			cfg:    OreConfig{Context: ""},
			wantOK: false,
		},
		{
			name:   "context_without_matching_node",
			cfg:    OreConfig{Context: "prod", Nodes: map[string]NodeConfig{}},
			wantOK: false,
		},
		{
			name: "node_with_empty_addr",
			cfg: OreConfig{
				Context: "prod",
				Nodes:   map[string]NodeConfig{"prod": {Token: "tok", Project: "proj"}},
			},
			wantToken:   "tok",
			wantProject: "proj",
			wantOK:      false,
		},
		{
			name: "resolved",
			cfg: OreConfig{
				Context: "prod",
				Nodes: map[string]NodeConfig{
					"prod": {Addr: "1.2.3.4:9090", Token: "tok", Project: "proj"},
				},
			},
			wantAddr:    "1.2.3.4:9090",
			wantToken:   "tok",
			wantProject: "proj",
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			addr, token, project, ok := ResolveRemote(&tt.cfg)
			assert.Equal(t, tt.wantAddr, addr)
			assert.Equal(t, tt.wantToken, token)
			assert.Equal(t, tt.wantProject, project)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestPathHelpers(t *testing.T) {
	configHome, dataHome := setupXDG(t)

	assert.Equal(t, filepath.Join(configHome, "ore"), OreConfigDir())
	assert.Equal(t, filepath.Join(configHome, "ored"), OredConfigDir())
	assert.Equal(t, filepath.Join(dataHome, "ored"), OredDataDir())
	assert.Equal(t, filepath.Join(dataHome, "ored", "projects"), OredProjectsDir())
	assert.Equal(t, filepath.Join(configHome, "ore", "config.yaml"), OreConfigFile())
	assert.Equal(t, filepath.Join(configHome, "ored", "config.yaml"), OredConfigFile())
}

func newSearchViper(dir string) *viper.Viper {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)
	return v
}

func TestReadOrCreateConfigCreatesFile(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "config.yaml")

	v := newSearchViper(dir)
	require.NoError(t, readOrCreateConfig(v, path))
	assert.FileExists(t, path)
}

func TestReadOrCreateConfigReadsExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("log_level: debug\n"), 0o600))

	v := newSearchViper(dir)
	require.NoError(t, readOrCreateConfig(v, path))
	assert.Equal(t, "debug", v.GetString("log_level"))
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	tok1, err := generateToken()
	require.NoError(t, err)
	assert.Len(t, tok1, 64)
	assert.Regexp(t, "^[0-9a-f]{64}$", tok1)

	tok2, err := generateToken()
	require.NoError(t, err)
	assert.NotEqual(t, tok1, tok2)
}

func TestLoadOredDefaults(t *testing.T) {
	configHome, dataHome := setupXDG(t)

	cfg, err := LoadOred()
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.Addr)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Empty(t, cfg.Token)
	assert.False(t, cfg.BindMounts)
	assert.Equal(t, filepath.Join(dataHome, "ored", "projects"), cfg.Projects)

	assert.FileExists(t, filepath.Join(configHome, "ored", "config.yaml"))
	assert.DirExists(t, cfg.Projects)
}

func TestLoadOredReadsFile(t *testing.T) {
	configHome, _ := setupXDG(t)
	oredDir := filepath.Join(configHome, "ored")
	require.NoError(t, os.MkdirAll(oredDir, 0o755))
	contents := "addr: \":1234\"\nlog_level: debug\ntoken: existing\nbind_mounts: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(oredDir, "config.yaml"), []byte(contents), 0o600))

	cfg, err := LoadOred()
	require.NoError(t, err)

	assert.Equal(t, ":1234", cfg.Addr)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "existing", cfg.Token)
	assert.True(t, cfg.BindMounts)
}

func TestLoadOredEnvOverride(t *testing.T) {
	setupXDG(t)
	t.Setenv("ORED_ADDR", ":7777")
	t.Setenv("ORED_TOKEN", "envtoken")

	cfg, err := LoadOred()
	require.NoError(t, err)

	assert.Equal(t, ":7777", cfg.Addr)
	assert.Equal(t, "envtoken", cfg.Token)
}

func TestEnsureToken(t *testing.T) {
	configHome, _ := setupXDG(t)

	cfg, err := LoadOred()
	require.NoError(t, err)
	require.Empty(t, cfg.Token)

	require.NoError(t, EnsureToken(cfg))
	first := cfg.Token
	assert.Len(t, first, 64)

	path := filepath.Join(configHome, "ored", "config.yaml")
	persisted := viper.New()
	persisted.SetConfigFile(path)
	require.NoError(t, persisted.ReadInConfig())
	assert.Equal(t, first, persisted.GetString("token"))

	require.NoError(t, EnsureToken(cfg))
	assert.Equal(t, first, cfg.Token)
}

func TestEnsureTokenKeepsExisting(t *testing.T) {
	setupXDG(t)

	cfg := &OredConfig{Token: "preset"}
	require.NoError(t, EnsureToken(cfg))
	assert.Equal(t, "preset", cfg.Token)
}

func TestOreLifecycle(t *testing.T) {
	configHome, _ := setupXDG(t)

	cfg, err := LoadOre(nil)
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Empty(t, cfg.Context)
	assert.Empty(t, cfg.Nodes)

	path := filepath.Join(configHome, "ore", "config.yaml")
	require.FileExists(t, path)

	t.Run("save_node", func(t *testing.T) {
		require.NoError(t, SaveNode("Prod", NodeConfig{
			Addr:    "1.2.3.4:9090",
			Token:   "tok",
			Project: "proj",
		}))

		got, err := LoadOre(nil)
		require.NoError(t, err)
		node, ok := got.Nodes["prod"]
		require.True(t, ok)
		assert.Equal(t, "1.2.3.4:9090", node.Addr)
		assert.Equal(t, "tok", node.Token)
		assert.Equal(t, "proj", node.Project)
	})

	t.Run("set_context_and_project", func(t *testing.T) {
		require.NoError(t, SetContext("Prod"))
		require.NoError(t, SetProject("world"))

		got, err := LoadOre(nil)
		require.NoError(t, err)
		assert.Equal(t, "prod", got.Context)
		assert.Equal(t, "world", got.Nodes["prod"].Project)

		addr, token, project, ok := ResolveRemote(got)
		assert.True(t, ok)
		assert.Equal(t, "1.2.3.4:9090", addr)
		assert.Equal(t, "tok", token)
		assert.Equal(t, "world", project)
	})

	t.Run("remove_missing_node", func(t *testing.T) {
		err := RemoveNode("ghost")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("remove_node_clears_context", func(t *testing.T) {
		require.NoError(t, RemoveNode("prod"))

		got, err := LoadOre(nil)
		require.NoError(t, err)
		assert.NotContains(t, got.Nodes, "prod")
		assert.Empty(t, got.Context)
	})

	t.Run("set_project_without_context", func(t *testing.T) {
		require.NoError(t, SetContext(""))

		err := SetProject("world")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no node selected")
	})

	t.Run("load_with_flags", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.String("log_level", "", "")
		require.NoError(t, flags.Set("log_level", "trace"))

		got, err := LoadOre(flags)
		require.NoError(t, err)
		assert.Equal(t, "trace", got.LogLevel)
	})
}
