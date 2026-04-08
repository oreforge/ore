package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManifest(t *testing.T) {
	t.Parallel()

	m := NewManifest()
	assert.NotNil(t, m.Binaries)
	assert.NotNil(t, m.Builds)
	assert.Empty(t, m.Binaries)
	assert.Empty(t, m.Builds)
}

func TestLoadManifest(t *testing.T) {
	t.Parallel()

	t.Run("file_not_found", func(t *testing.T) {
		t.Parallel()
		m := LoadManifest("/nonexistent/manifest.json")
		assert.NotNil(t, m.Binaries)
		assert.NotNil(t, m.Builds)
	})

	t.Run("valid_json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")

		m := &Manifest{
			Binaries: map[string]BinaryEntry{
				"paper:1.21": {SoftwareID: "paper:1.21", Filename: "paper.jar"},
			},
			Builds: map[string]Entry{
				"lobby": {ServerName: "lobby", ImageTag: "ore/lobby:abc123"},
			},
		}
		data, err := json.Marshal(m)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		loaded := LoadManifest(path)
		assert.Equal(t, "paper:1.21", loaded.Binaries["paper:1.21"].SoftwareID)
		assert.Equal(t, "ore/lobby:abc123", loaded.Builds["lobby"].ImageTag)
	})

	t.Run("nil_maps_initialized", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o644))

		m := LoadManifest(path)
		assert.NotNil(t, m.Binaries)
		assert.NotNil(t, m.Builds)
	})

	t.Run("invalid_json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))

		m := LoadManifest(path)
		assert.NotNil(t, m.Binaries)
		assert.NotNil(t, m.Builds)
	})
}

func TestManifestSave(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")

		original := NewManifest()
		original.Binaries["paper:1.21"] = BinaryEntry{
			SoftwareID: "paper:1.21",
			Filename:   "paper.jar",
			SHA256:     "abc123",
			Size:       1024,
			CachedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		original.Builds["lobby"] = Entry{
			ServerName: "lobby",
			ImageTag:   "ore/lobby:abc123",
			CacheKey:   "abc123def456",
		}

		require.NoError(t, original.Save(path))

		loaded := LoadManifest(path)
		assert.Equal(t, original.Binaries["paper:1.21"].SoftwareID, loaded.Binaries["paper:1.21"].SoftwareID)
		assert.Equal(t, original.Binaries["paper:1.21"].SHA256, loaded.Binaries["paper:1.21"].SHA256)
		assert.Equal(t, original.Builds["lobby"].ImageTag, loaded.Builds["lobby"].ImageTag)
	})

	t.Run("output_is_indented", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")

		m := NewManifest()
		m.Binaries["test"] = BinaryEntry{SoftwareID: "test"}
		require.NoError(t, m.Save(path))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "\n")
		assert.Contains(t, string(data), "  ")
	})
}
