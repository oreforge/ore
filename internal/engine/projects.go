package engine

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

	"github.com/oreforge/ore/internal/orchestrator"
	"github.com/oreforge/ore/internal/spec"
)

const defaultPollInterval = 5 * time.Minute

type ProjectManager struct {
	projectsDir string
	bindMounts  bool
	logger      *slog.Logger
	pollCancel  context.CancelFunc
	pollWg      sync.WaitGroup
}

func NewProjectManager(projectsDir string, bindMounts bool, logger *slog.Logger) *ProjectManager {
	return &ProjectManager{
		projectsDir: projectsDir,
		bindMounts:  bindMounts,
		logger:      logger,
	}
}

func (pm *ProjectManager) ProjectsDir() string {
	return pm.projectsDir
}

func (pm *ProjectManager) ListProjects() ([]string, error) {
	entries, err := os.ReadDir(pm.projectsDir)
	if err != nil {
		return nil, fmt.Errorf("reading projects directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		specFile := filepath.Join(pm.projectsDir, e.Name(), "ore.yaml")
		if _, statErr := os.Stat(specFile); statErr == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func (pm *ProjectManager) ResolveSpec(name string) (string, error) {
	if filepath.Base(name) != name {
		return "", fmt.Errorf("invalid project name")
	}
	specPath := filepath.Join(pm.projectsDir, name, "ore.yaml")
	if _, err := os.Stat(specPath); err != nil {
		return "", fmt.Errorf("project %q not found", name)
	}
	return specPath, nil
}

func (pm *ProjectManager) Pull(ctx context.Context, name string) error {
	projectDir := filepath.Join(pm.projectsDir, name)
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); err != nil {
		return fmt.Errorf("project %s is not a git repository", name)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", projectDir, "pull")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (pm *ProjectManager) Up(ctx context.Context, name string, opts UpOptions, logger *slog.Logger) error {
	specPath, err := pm.ResolveSpec(name)
	if err != nil {
		return err
	}
	eng := NewLocal(logger, specPath, WithBindMounts(pm.bindMounts))
	defer func() { _ = eng.Close() }()
	return eng.Up(ctx, opts)
}

func (pm *ProjectManager) Down(ctx context.Context, name string, logger *slog.Logger) error {
	specPath, err := pm.ResolveSpec(name)
	if err != nil {
		return err
	}
	eng := NewLocal(logger, specPath, WithBindMounts(pm.bindMounts))
	defer func() { _ = eng.Close() }()
	return eng.Down(ctx)
}

func (pm *ProjectManager) Build(ctx context.Context, name string, noCache bool, logger *slog.Logger) error {
	specPath, err := pm.ResolveSpec(name)
	if err != nil {
		return err
	}
	eng := NewLocal(logger, specPath, WithBindMounts(pm.bindMounts))
	defer func() { _ = eng.Close() }()
	return eng.Build(ctx, noCache)
}

func (pm *ProjectManager) Status(ctx context.Context, name string) (*orchestrator.NetworkStatus, error) {
	specPath, err := pm.ResolveSpec(name)
	if err != nil {
		return nil, err
	}
	eng := NewLocal(pm.logger, specPath, WithBindMounts(pm.bindMounts))
	defer func() { _ = eng.Close() }()
	return eng.Status(ctx)
}

func (pm *ProjectManager) Prune(ctx context.Context, name string, target PruneTarget, logger *slog.Logger) error {
	specPath, err := pm.ResolveSpec(name)
	if err != nil {
		return err
	}
	eng := NewLocal(logger, specPath, WithBindMounts(pm.bindMounts))
	defer func() { _ = eng.Close() }()
	return eng.Prune(ctx, target)
}

func (pm *ProjectManager) Clean(ctx context.Context, name string, target CleanTarget, logger *slog.Logger) error {
	specPath, err := pm.ResolveSpec(name)
	if err != nil {
		return err
	}
	eng := NewLocal(logger, specPath, WithBindMounts(pm.bindMounts))
	defer func() { _ = eng.Close() }()
	return eng.Clean(ctx, target)
}

func (pm *ProjectManager) Deploy(ctx context.Context, name string) error {
	pm.logger.Info("deploying project", "project", name)

	if err := pm.Pull(ctx, name); err != nil {
		return fmt.Errorf("pulling %s: %w", name, err)
	}

	if err := pm.Up(ctx, name, UpOptions{}, pm.logger); err != nil {
		return fmt.Errorf("deploying %s: %w", name, err)
	}

	pm.logger.Info("deploy complete", "project", name)
	return nil
}

func (pm *ProjectManager) StartPolling() {
	ctx, cancel := context.WithCancel(context.Background())
	pm.pollCancel = cancel

	names, err := pm.ListProjects()
	if err != nil {
		pm.logger.Error("failed to list projects for polling", "error", err)
		return
	}

	for _, name := range names {
		specPath, err := pm.ResolveSpec(name)
		if err != nil {
			continue
		}
		s, err := spec.Load(specPath)
		if err != nil {
			pm.logger.Warn("failed to load spec for polling", "project", name, "error", err)
			continue
		}
		if s.GitOps == nil || !s.GitOps.Poll.Enabled {
			continue
		}

		interval := s.GitOps.Poll.Interval
		if interval <= 0 {
			interval = defaultPollInterval
		}

		pm.pollWg.Add(1)
		go pm.poll(ctx, name, interval)
	}
}

func (pm *ProjectManager) StopPolling() {
	if pm.pollCancel != nil {
		pm.pollCancel()
	}
	pm.pollWg.Wait()
}

func (pm *ProjectManager) poll(ctx context.Context, name string, interval time.Duration) {
	defer pm.pollWg.Done()

	logger := pm.logger.With("project", name)
	logger.Info("gitops polling started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pm.Deploy(ctx, name); err != nil {
				logger.Error("gitops deploy failed", "error", err)
			}
		}
	}
}

func (pm *ProjectManager) Shutdown(ctx context.Context) {
	names, err := pm.ListProjects()
	if err != nil {
		pm.logger.Error("failed to list projects for shutdown", "error", err)
		return
	}
	for _, name := range names {
		pm.logger.Info("stopping project", "project", name)
		if err := pm.Down(ctx, name, pm.logger); err != nil {
			pm.logger.Error("failed to stop project", "project", name, "error", err)
		}
	}
}
