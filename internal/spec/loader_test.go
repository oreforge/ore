package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		env     map[string]string
		want    string
		wantErr string
	}{
		{
			name:  "single_var",
			input: "${FOO}",
			env:   map[string]string{"FOO": "bar"},
			want:  "bar",
		},
		{
			name:  "multiple_vars",
			input: "${A} and ${B}",
			env:   map[string]string{"A": "hello", "B": "world"},
			want:  "hello and world",
		},
		{
			name:    "unset_var",
			input:   "${MISSING_ORE_TEST_VAR}",
			wantErr: "MISSING_ORE_TEST_VAR",
		},
		{
			name:  "no_vars",
			input: "plain text",
			want:  "plain text",
		},
		{
			name:  "empty_value",
			input: "${EMPTY}",
			env:   map[string]string{"EMPTY": ""},
			want:  "",
		},
		{
			name:  "dollar_without_braces_untouched",
			input: "$FOO",
			want:  "$FOO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := expandEnvVars(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("valid_yaml", func(t *testing.T) {
		dir := t.TempDir()
		content := `
network: test
servers:
  - name: lobby
    dir: ./lobby
    software: paper:1.21
`
		path := filepath.Join(dir, "ore.yaml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		net, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, "test", net.Network)
		require.Len(t, net.Servers, 1)
		assert.Equal(t, "lobby", net.Servers[0].Name)
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := Load("/nonexistent/ore.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading spec")
	})

	t.Run("invalid_yaml_syntax", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ore.yaml")
		require.NoError(t, os.WriteFile(path, []byte(":\n  :\n  - [invalid"), 0o644))

		_, err := Load(path)
		require.Error(t, err)
	})

	t.Run("validation_failure", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ore.yaml")
		require.NoError(t, os.WriteFile(path, []byte("network: test\nservers: []\n"), 0o644))

		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one server is required")
	})

	t.Run("env_var_expansion", func(t *testing.T) {
		t.Setenv("ORE_TEST_NET", "my-network")
		dir := t.TempDir()
		content := `
network: ${ORE_TEST_NET}
servers:
  - name: lobby
    dir: ./lobby
    software: paper:1.21
`
		path := filepath.Join(dir, "ore.yaml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		net, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, "my-network", net.Network)
	})

	t.Run("unset_env_var", func(t *testing.T) {
		dir := t.TempDir()
		content := `
network: ${ORE_UNSET_VAR_12345}
servers:
  - name: lobby
    dir: ./lobby
    software: paper:1.21
`
		path := filepath.Join(dir, "ore.yaml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ORE_UNSET_VAR_12345")
	})
}
