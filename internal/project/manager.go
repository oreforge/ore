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

const (
	defaultPollInterval = 5 * time.Minute
	gitTimeout          = 30 * time.Second
)

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
	deployMu    sync.Mutex
	deploying   map[string]bool
}

func NewManager(projectsDir string, bindMounts bool, logger *slog.Logger) *Manager {
	return &Manager{
		projectsDir: projectsDir,
		bindMounts:  bindMounts,
		logger:      logger,
		polls:       make(map[string]pollEntry),
		deploying:   make(map[string]bool),
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
	m.logger.Debug("pulling latest changes", "project", name)
	projectDir := filepath.Join(m.projectsDir, name)
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); err != nil {
		return fmt.Errorf("project %s is not a git repository", name)
	}
	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(gitCtx, "git", "-C", projectDir, "pull")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) hasRemoteChanges(ctx context.Context, name string) (bool, error) {
	projectDir := filepath.Join(m.projectsDir, name)
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); err != nil {
		return false, fmt.Errorf("project %s is not a git repository", name)
	}

	fetchCtx, fetchCancel := context.WithTimeout(ctx, gitTimeout)
	defer fetchCancel()
	fetchCmd := exec.CommandContext(fetchCtx, "git", "-C", projectDir, "fetch")
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("git fetch failed: %s", strings.TrimSpace(string(output)))
	}

	localCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "HEAD")
	localOut, err := localCmd.Output()
	if err != nil {
		return false, fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}

	remoteCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "@{u}")
	remoteOut, err := remoteCmd.Output()
	if err != nil {
		return false, fmt.Errorf("git rev-parse @{u} failed: %w", err)
	}

	local := strings.TrimSpace(string(localOut))
	remote := strings.TrimSpace(string(remoteOut))
	return local != remote, nil
}
