package project

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validSpec(network string) string {
	return "network: " + network + "\n" +
		"servers:\n" +
		"  - name: lobby\n" +
		"    dir: ./servers/lobby\n" +
		"    software: paper:1.21\n"
}

func specWithIcon(network, icon string) string {
	return validSpec(network) + "icon: " + icon + "\n"
}

func writeProject(t *testing.T, dir, name, contents string) string {
	t.Helper()
	projectDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	specPath := filepath.Join(projectDir, "ore.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte(contents), 0o644))
	return projectDir
}

func newTestManager(t *testing.T, projectsDir string) *Manager {
	t.Helper()
	return NewManager(projectsDir, false, testLogger(), nil)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	writeProject(t, projectsDir, "alpha", validSpec("alpha"))
	m := newTestManager(t, projectsDir)

	tests := []struct {
		name        string
		project     string
		wantErr     string
		wantRelPath string
	}{
		{
			name:        "existing_project",
			project:     "alpha",
			wantRelPath: filepath.Join("alpha", "ore.yaml"),
		},
		{
			name:    "unknown_project",
			project: "missing",
			wantErr: "not found",
		},
		{
			name:    "path_traversal_parent",
			project: "../alpha",
			wantErr: "invalid project name",
		},
		{
			name:    "path_traversal_nested",
			project: "a/b",
			wantErr: "invalid project name",
		},
		{
			name:    "dot",
			project: ".",
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := m.Resolve(tt.project)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(projectsDir, tt.wantRelPath), got)
		})
	}
}

func TestList(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	writeProject(t, projectsDir, "alpha", validSpec("alpha"))
	writeProject(t, projectsDir, "beta", validSpec("beta"))

	require.NoError(t, os.MkdirAll(filepath.Join(projectsDir, "no-spec"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectsDir, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectsDir, ".hidden", "ore.yaml"), []byte(validSpec("hidden")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectsDir, "loose.yaml"), []byte(validSpec("loose")), 0o644))

	m := newTestManager(t, projectsDir)

	names, err := m.List()
	require.NoError(t, err)
	sort.Strings(names)
	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestListMissingDir(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, filepath.Join(t.TempDir(), "does-not-exist"))

	names, err := m.List()
	require.Error(t, err)
	assert.Nil(t, names)
}

func TestIconPath(t *testing.T) {
	t.Parallel()

	t.Run("no_icon_configured", func(t *testing.T) {
		t.Parallel()
		projectsDir := t.TempDir()
		writeProject(t, projectsDir, "alpha", validSpec("alpha"))
		m := newTestManager(t, projectsDir)

		got, err := m.IconPath("alpha")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no icon configured")
		assert.Empty(t, got)
	})

	t.Run("valid_in_repo_icon", func(t *testing.T) {
		t.Parallel()
		projectsDir := t.TempDir()
		projectDir := writeProject(t, projectsDir, "alpha", specWithIcon("alpha", "./icon.png"))
		iconPath := filepath.Join(projectDir, "icon.png")
		require.NoError(t, os.WriteFile(iconPath, []byte("fake"), 0o644))
		m := newTestManager(t, projectsDir)

		got, err := m.IconPath("alpha")
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(got))
		wantAbs, absErr := filepath.Abs(iconPath)
		require.NoError(t, absErr)
		assert.Equal(t, wantAbs, got)
	})

	t.Run("nested_in_repo_icon", func(t *testing.T) {
		t.Parallel()
		projectsDir := t.TempDir()
		projectDir := writeProject(t, projectsDir, "alpha", specWithIcon("alpha", "./assets/icon.png"))
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "assets"), 0o755))
		iconPath := filepath.Join(projectDir, "assets", "icon.png")
		require.NoError(t, os.WriteFile(iconPath, []byte("fake"), 0o644))
		m := newTestManager(t, projectsDir)

		got, err := m.IconPath("alpha")
		require.NoError(t, err)
		wantAbs, absErr := filepath.Abs(iconPath)
		require.NoError(t, absErr)
		assert.Equal(t, wantAbs, got)
	})

	t.Run("icon_escapes_project_directory", func(t *testing.T) {
		t.Parallel()
		projectsDir := t.TempDir()
		writeProject(t, projectsDir, "alpha", specWithIcon("alpha", "../secret.png"))
		m := newTestManager(t, projectsDir)

		got, err := m.IconPath("alpha")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "icon path must point to a file")
		assert.Empty(t, got)
	})

	t.Run("absolute_icon_rejected", func(t *testing.T) {
		t.Parallel()
		projectsDir := t.TempDir()
		writeProject(t, projectsDir, "alpha", specWithIcon("alpha", "/etc/passwd"))
		m := newTestManager(t, projectsDir)

		got, err := m.IconPath("alpha")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "icon path must be relative")
		assert.Empty(t, got)
	})

	t.Run("unknown_project", func(t *testing.T) {
		t.Parallel()
		m := newTestManager(t, t.TempDir())

		got, err := m.IconPath("missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Empty(t, got)
	})
}
