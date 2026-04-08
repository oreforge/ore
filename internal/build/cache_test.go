package build

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheKey(t *testing.T) {
	t.Parallel()

	hexPattern := regexp.MustCompile(`^[0-9a-f]{12}$`)

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "server.properties"), []byte("motd=test"), 0o644))

		k1, err := CacheKey("paper:1.21", "build1", dir)
		require.NoError(t, err)
		k2, err := CacheKey("paper:1.21", "build1", dir)
		require.NoError(t, err)

		assert.Equal(t, k1, k2)
		assert.Regexp(t, hexPattern, k1)
	})

	t.Run("different_software_id", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), []byte("a"), 0o644))

		k1, err := CacheKey("paper:1.21", "build1", dir)
		require.NoError(t, err)
		k2, err := CacheKey("velocity:3.3", "build1", dir)
		require.NoError(t, err)

		assert.NotEqual(t, k1, k2)
	})

	t.Run("different_file_content", func(t *testing.T) {
		t.Parallel()
		dir1 := t.TempDir()
		dir2 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir1, "config.yml"), []byte("v1"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir2, "config.yml"), []byte("v2"), 0o644))

		k1, err := CacheKey("paper:1.21", "build1", dir1)
		require.NoError(t, err)
		k2, err := CacheKey("paper:1.21", "build1", dir2)
		require.NoError(t, err)

		assert.NotEqual(t, k1, k2)
	})
}

func TestHashDirectory(t *testing.T) {
	t.Parallel()

	t.Run("empty_directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		h1, err := hashDirectory(dir)
		require.NoError(t, err)
		h2, err := hashDirectory(dir)
		require.NoError(t, err)
		assert.Equal(t, h1, h2)
	})

	t.Run("sorted_order_deterministic", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))

		h1, err := hashDirectory(dir)
		require.NoError(t, err)

		dir2 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("a"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("b"), 0o644))

		h2, err := hashDirectory(dir2)
		require.NoError(t, err)

		assert.Equal(t, h1, h2)
	})

	t.Run("dotfiles_skipped", func(t *testing.T) {
		t.Parallel()
		dir1 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir1, "config.yml"), []byte("data"), 0o644))

		dir2 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir2, "config.yml"), []byte("data"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir2, ".hidden"), []byte("secret"), 0o644))

		h1, err := hashDirectory(dir1)
		require.NoError(t, err)
		h2, err := hashDirectory(dir2)
		require.NoError(t, err)

		assert.Equal(t, h1, h2)
	})

	t.Run("dot_directories_skipped", func(t *testing.T) {
		t.Parallel()
		dir1 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir1, "config.yml"), []byte("data"), 0o644))

		dir2 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir2, "config.yml"), []byte("data"), 0o644))
		gitDir := filepath.Join(dir2, ".git")
		require.NoError(t, os.MkdirAll(gitDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref"), 0o644))

		h1, err := hashDirectory(dir1)
		require.NoError(t, err)
		h2, err := hashDirectory(dir2)
		require.NoError(t, err)

		assert.Equal(t, h1, h2)
	})

	t.Run("nested_subdirectories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		sub := filepath.Join(dir, "plugins", "essentials")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "config.yml"), []byte("data"), 0o644))

		h, err := hashDirectory(dir)
		require.NoError(t, err)
		assert.NotEmpty(t, h)
	})
}
