package engine

import (
	"context"
	"errors"
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
	specPath   string
	logger     *slog.Logger
	registry   *providers.Registry
	bindMounts bool
}

type LocalOption func(*Local)

func WithBindMounts(enabled bool) LocalOption {
	return func(l *Local) { l.bindMounts = enabled }
}

func NewLocal(logger *slog.Logger, specPath string, opts ...LocalOption) *Local {
	l := &Local{
		specPath:   specPath,
		logger:     logger,
		registry:   providers.NewDefault(),
		bindMounts: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

func (l *Local) Up(ctx context.Context, noCache bool) error {
	br, err := l.doBuild(ctx, build.Options{NoCache: noCache})
	if err != nil {
		return err
	}
	defer func() { _ = br.docker.Close() }()

	orch := orchestrator.New(br.docker, l.logger, br.cache, l.bindMounts)
	return orch.Up(ctx, br.spec, br.images)
}

func (l *Local) Down(ctx context.Context) error {
	s, err := spec.Load(l.specPath)
	if err != nil {
		return err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	orch := orchestrator.New(dockerClient, l.logger, nil, l.bindMounts)
	return orch.Down(ctx, s)
}

func (l *Local) Build(ctx context.Context, noCache bool) error {
	br, err := l.doBuild(ctx, build.Options{NoCache: noCache, ForceBuild: true})
	if err != nil {
		return err
	}
	_ = br.docker.Close()
	return nil
}

func (l *Local) Status(ctx context.Context) (*orchestrator.NetworkStatus, error) {
	s, err := spec.Load(l.specPath)
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	orch := orchestrator.New(dockerClient, l.logger, nil, l.bindMounts)
	return orch.Status(ctx, s)
}

func (l *Local) Prune(ctx context.Context, target PruneTarget) error {
	s, err := spec.Load(l.specPath)
	if err != nil {
		return err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	orch := orchestrator.New(dockerClient, l.logger, nil, l.bindMounts)

	switch target {
	case PruneAll:
		var errs []error
		if err := orch.Down(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("stopping containers: %w", err))
		}
		if err := orch.PruneImages(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("pruning images: %w", err))
		}
		if err := orch.PruneVolumes(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("pruning volumes: %w", err))
		}
		repoRoot := filepath.Dir(l.specPath)
		if mgr, wdErr := cache.New(repoRoot, l.logger); wdErr == nil {
			if cleanErr := mgr.Clean(); cleanErr != nil {
				errs = append(errs, fmt.Errorf("cleaning .ore directory: %w", cleanErr))
			}
		}
		l.logger.Info("pruned all resources")
		return errors.Join(errs...)
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

func (l *Local) Clean(ctx context.Context, target CleanTarget) error {
	repoRoot := filepath.Dir(l.specPath)
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

func (l *Local) Console(ctx context.Context, serverName string, replica int) error {
	s, err := spec.Load(l.specPath)
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

	fmt.Fprintln(os.Stderr, "attached to console (press ctrl+c to detach)")
	cmd := exec.CommandContext(ctx, "docker", "attach", "--detach-keys=ctrl-c", "--sig-proxy=false", containerName)
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

func (l *Local) doBuild(ctx context.Context, opts build.Options) (*buildResult, error) {
	s, err := spec.Load(l.specPath)
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	repoRoot := filepath.Dir(l.specPath)
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
