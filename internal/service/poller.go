package service

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

type Poller struct {
	cfg    *config.OredConfig
	logger *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

func NewPoller(cfg *config.OredConfig, logger *slog.Logger) *Poller {
	return &Poller{
		cfg:    cfg,
		logger: logger,
		done:   make(chan struct{}),
	}
}

func (p *Poller) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	ticker := time.NewTicker(p.cfg.GitPoll.Interval)

	go func() {
		defer close(p.done)
		defer ticker.Stop()

		p.logger.Info("git poller started",
			"interval", p.cfg.GitPoll.Interval,
			"on_update", p.cfg.GitPoll.OnUpdate,
		)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.pollAll(ctx)
			}
		}
	}()
}

func (p *Poller) Stop() {
	p.cancel()
	<-p.done
	p.logger.Info("git poller stopped")
}

func (p *Poller) pollAll(ctx context.Context) {
	entries, err := os.ReadDir(p.cfg.Projects)
	if err != nil {
		p.logger.Error("polling: failed to read projects directory", "error", err)
		return
	}

	for _, e := range entries {
		if ctx.Err() != nil {
			return
		}
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		projectDir := filepath.Join(p.cfg.Projects, e.Name())
		specPath := filepath.Join(projectDir, "ore.yaml")
		gitDir := filepath.Join(projectDir, ".git")

		if _, err := os.Stat(specPath); err != nil {
			continue
		}
		if _, err := os.Stat(gitDir); err != nil {
			continue
		}

		p.pollProject(ctx, e.Name(), projectDir, specPath)
	}
}

func (p *Poller) pollProject(ctx context.Context, name, projectDir, specPath string) {
	logger := p.logger.With("project", name)

	before, err := gitRevParse(ctx, projectDir)
	if err != nil {
		logger.Error("polling: failed to get current commit", "error", err)
		return
	}

	pullCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "pull")
	if output, pullErr := pullCmd.CombinedOutput(); pullErr != nil {
		logger.Error("polling: git pull failed", "error", strings.TrimSpace(string(output)))
		return
	}

	after, err := gitRevParse(ctx, projectDir)
	if err != nil {
		logger.Error("polling: failed to get new commit", "error", err)
		return
	}

	if before == after {
		logger.Debug("polling: no changes", "commit", before)
		return
	}

	logger.Info("polling: project updated", "from", before, "to", after)

	if p.cfg.GitPoll.OnUpdate == "deploy" {
		p.deploy(ctx, logger, specPath)
	}
}

func (p *Poller) deploy(ctx context.Context, logger *slog.Logger, specPath string) {
	logger.Info("polling: deploying")

	eng := engine.NewLocal(logger, specPath, engine.WithBindMounts(p.cfg.BindMounts))
	defer func() { _ = eng.Close() }()

	if err := eng.Down(ctx); err != nil {
		logger.Error("polling: down failed", "error", err)
	}

	if err := eng.Up(ctx, false); err != nil {
		logger.Error("polling: up failed", "error", err)
		return
	}

	logger.Info("polling: deploy complete")
}

func gitRevParse(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
