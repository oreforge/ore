package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/cache"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/orchestrator"
	"github.com/oreforge/ore/internal/resolver/providers"
	"github.com/oreforge/ore/internal/spec"
)

type Local struct {
	logger   *slog.Logger
	registry *providers.Registry
}

func NewLocal(logger *slog.Logger) *Local {
	return &Local{
		logger:   logger,
		registry: providers.NewDefault(),
	}
}

func (l *Local) Up(ctx context.Context, specPath string, noCache bool) error {
	br, err := l.doBuild(ctx, specPath, build.Options{NoCache: noCache})
	if err != nil {
		return err
	}
	defer func() { _ = br.docker.Close() }()

	orch := orchestrator.New(br.docker, l.logger, br.cache)
	return orch.Up(ctx, br.spec, br.images)
}

func (l *Local) Down(ctx context.Context, specPath string) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	orch := orchestrator.New(dockerClient, l.logger, nil)
	return orch.Down(ctx, s)
}

func (l *Local) Build(ctx context.Context, specPath string, noCache bool) error {
	br, err := l.doBuild(ctx, specPath, build.Options{NoCache: noCache, ForceBuild: true})
	if err != nil {
		return err
	}
	_ = br.docker.Close()
	return nil
}

func (l *Local) Status(ctx context.Context, specPath string) (*orchestrator.NetworkStatus, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	orch := orchestrator.New(dockerClient, l.logger, nil)
	return orch.Status(ctx, s)
}

func (l *Local) Prune(ctx context.Context, specPath string, target PruneTarget) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	orch := orchestrator.New(dockerClient, l.logger, nil)

	switch target {
	case PruneAll:
		if err := orch.Down(ctx, s); err != nil {
			l.logger.Warn("failed to stop containers", "error", err)
		}
		if err := orch.PruneImages(ctx, s); err != nil {
			l.logger.Warn("failed to prune images", "error", err)
		}
		if err := orch.PruneVolumes(ctx, s); err != nil {
			l.logger.Warn("failed to prune volumes", "error", err)
		}
		repoRoot := filepath.Dir(specPath)
		mgr, wdErr := cache.New(repoRoot, l.logger)
		if wdErr == nil {
			if cleanErr := mgr.Clean(); cleanErr != nil {
				l.logger.Warn("failed to clean .ore directory", "error", cleanErr)
			}
		}
		l.logger.Info("pruned all resources")
		return nil
	case PruneContainers:
		return orch.Down(ctx, s)
	case PruneImages:
		return orch.PruneImages(ctx, s)
	case PruneVolumes:
		return orch.PruneVolumes(ctx, s)
	default:
		return fmt.Errorf("unknown prune target: %d", target)
	}
}

func (l *Local) Clean(ctx context.Context, specPath string, target CleanTarget) error {
	repoRoot := filepath.Dir(specPath)
	mgr, err := cache.New(repoRoot, l.logger)
	if err != nil {
		return fmt.Errorf("opening .ore directory: %w", err)
	}

	switch target {
	case CleanAll:
		return mgr.Clean()
	case CleanCache:
		return mgr.CleanCache()
	case CleanBuilds:
		return mgr.CleanBuilds()
	default:
		return fmt.Errorf("unknown clean target: %d", target)
	}
}

func (l *Local) Console(ctx context.Context, specPath string, serverName string, replica int) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return err
	}

	var srv *spec.ServerSpec
	for i := range s.Servers {
		if s.Servers[i].Name == serverName {
			srv = &s.Servers[i]
			break
		}
	}
	if srv == nil {
		return fmt.Errorf("server %q not found in ore.yaml", serverName)
	}

	containerName := serverName
	if srv.EffectiveReplicas() > 1 {
		containerName = fmt.Sprintf("%s-%d", serverName, replica)
	}

	cmd := exec.CommandContext(ctx, "docker", "attach", containerName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (l *Local) Close() error {
	return nil
}

type buildResult struct {
	images map[string]build.Result
	spec   *spec.NetworkSpec
	docker docker.Client
	cache  *cache.Manager
}

func (l *Local) doBuild(ctx context.Context, specPath string, opts build.Options) (*buildResult, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	repoRoot := filepath.Dir(specPath)
	mgr, err := cache.New(repoRoot, l.logger)
	if err != nil {
		_ = dockerClient.Close()
		return nil, fmt.Errorf("initializing .ore directory: %w", err)
	}

	builder := build.NewBuilder(dockerClient, l.registry, l.logger, mgr, opts)
	images, err := builder.BuildAll(ctx, s, repoRoot)
	if err != nil {
		_ = dockerClient.Close()
		return nil, err
	}

	for name, res := range images {
		l.logger.Info("built image", "server", name, "tag", res.ImageTag)
	}

	return &buildResult{
		images: images,
		spec:   s,
		docker: dockerClient,
		cache:  mgr,
	}, nil
}
