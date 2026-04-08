package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeployState(t *testing.T) {
	t.Parallel()

	s := NewDeployState()
	assert.NotNil(t, s.Servers)
	assert.NotNil(t, s.Services)
	assert.Empty(t, s.Servers)
	assert.Empty(t, s.Services)
}

func TestLoadState(t *testing.T) {
	t.Parallel()

	t.Run("no_file", func(t *testing.T) {
		t.Parallel()
		s := LoadState(t.TempDir())
		assert.NotNil(t, s.Servers)
		assert.NotNil(t, s.Services)
	})

	t.Run("valid_json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		data := `{
  "servers": {"lobby": {"image_tag": "ore/lobby:abc", "config_hash": "hash1"}},
  "services": {"db": {"image": "postgres:16", "config_hash": "hash2"}}
}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(data), 0o644))

		s := LoadState(dir)
		assert.Equal(t, "ore/lobby:abc", s.Servers["lobby"].ImageTag)
		assert.Equal(t, "hash1", s.Servers["lobby"].ConfigHash)
		assert.Equal(t, "postgres:16", s.Services["db"].Image)
	})

	t.Run("nil_maps_initialized", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{}`), 0o644))

		s := LoadState(dir)
		assert.NotNil(t, s.Servers)
		assert.NotNil(t, s.Services)
	})

	t.Run("invalid_json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte("not json"), 0o644))

		s := LoadState(dir)
		assert.NotNil(t, s.Servers)
		assert.NotNil(t, s.Services)
	})
}

func TestSaveState(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		original := NewDeployState()
		original.Servers["lobby"] = ServerState{ImageTag: "ore/lobby:abc", ConfigHash: "hash1"}
		original.Services["db"] = ServiceState{Image: "postgres:16", ConfigHash: "hash2"}

		require.NoError(t, SaveState(dir, original))

		loaded := LoadState(dir)
		assert.Equal(t, original.Servers["lobby"], loaded.Servers["lobby"])
		assert.Equal(t, original.Services["db"], loaded.Services["db"])
	})

	t.Run("no_tmp_file_left", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, SaveState(dir, NewDeployState()))

		_, err := os.Stat(filepath.Join(dir, "state.json.tmp"))
		assert.True(t, os.IsNotExist(err))

		_, err = os.Stat(filepath.Join(dir, "state.json"))
		assert.NoError(t, err)
	})
}
