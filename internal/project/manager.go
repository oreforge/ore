package project

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultPollInterval = 5 * time.Minute

type pollEntry struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Manager struct {
	projectsDir string
	bindMounts  bool
	logger      *slog.Logger
	pollCtx     context.Context
	pollCancel  context.CancelFunc
	pollMu      sync.Mutex
	polls       map[string]pollEntry
}

func NewManager(projectsDir string, bindMounts bool, logger *slog.Logger) *Manager {
	return &Manager{
		projectsDir: projectsDir,
		bindMounts:  bindMounts,
		logger:      logger,
		polls:       make(map[string]pollEntry),
	}
}

func (m *Manager) ProjectsDir() string {
	return m.projectsDir
}

func (m *Manager) List() ([]string, error) {
	entries, err := os.ReadDir(m.projectsDir)
	if err != nil {
		return nil, fmt.Errorf("reading projects directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		specFile := filepath.Join(m.projectsDir, e.Name(), "ore.yaml")
		if _, statErr := os.Stat(specFile); statErr == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func (m *Manager) Resolve(name string) (string, error) {
	if filepath.Base(name) != name {
		return "", fmt.Errorf("invalid project name")
	}
	specPath := filepath.Join(m.projectsDir, name, "ore.yaml")
	if _, err := os.Stat(specPath); err != nil {
		return "", fmt.Errorf("project %q not found", name)
	}
	return specPath, nil
}

func (m *Manager) Pull(ctx context.Context, name string) error {
	projectDir := filepath.Join(m.projectsDir, name)
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); err != nil {
		return fmt.Errorf("project %s is not a git repository", name)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", projectDir, "pull")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
