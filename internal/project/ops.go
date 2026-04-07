package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/software/providers"
	"github.com/oreforge/ore/internal/spec"
)

type PruneTarget int

const (
	PruneAll PruneTarget = iota
	PruneContainers
	PruneImages
	PruneVolumes
)

type CleanTarget int

const (
	CleanAll CleanTarget = iota
	CleanCache
	CleanBuilds
)

type UpOptions struct {
	NoCache bool
	Force   bool
}

func (t PruneTarget) String() string {
	switch t {
	case PruneAll:
		return "all"
	case PruneContainers:
		return "servers"
	case PruneImages:
		return "images"
	case PruneVolumes:
		return "data"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

func (t CleanTarget) String() string {
	switch t {
	case CleanAll:
		return "all"
	case CleanCache:
		return "cache"
	case CleanBuilds:
		return "builds"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

type buildResult struct {
	images  map[string]build.Result
	spec    *spec.Network
	docker  docker.Client
	workDir *build.WorkDir
}

func (m *Manager) doBuild(ctx context.Context, specPath string, opts build.Options, logger *slog.Logger) (*buildResult, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	ok := false
	defer func() {
		if !ok {
			_ = dockerClient.Close()
		}
	}()

	repoRoot := filepath.Dir(specPath)
	wd, err := build.NewWorkDir(repoRoot, logger)
	if err != nil {
		return nil, fmt.Errorf("initializing .ore directory: %w", err)
	}

	builder := build.NewBuilder(dockerClient, providers.New(), logger, wd, opts)
	images, err := builder.BuildAll(ctx, s, repoRoot)
	if err != nil {
		return nil, err
	}

	for name, res := range images {
		logger.Info("built image", "server", name, "tag", res.ImageTag)
	}

	ok = true
	return &buildResult{
		images:  images,
		spec:    s,
		docker:  dockerClient,
		workDir: wd,
	}, nil
}

func (m *Manager) Up(ctx context.Context, name string, opts UpOptions, logger *slog.Logger) error {
	specPath, err := m.Resolve(name)
	if err != nil {
		return err
	}

	br, err := m.doBuild(ctx, specPath, build.Options{NoCache: opts.NoCache}, logger)
	if err != nil {
		return err
	}
	defer func() { _ = br.docker.Close() }()

	var prevState *deploy.State
	if !opts.Force && br.workDir != nil {
		prevState = deploy.LoadState(br.workDir.Root())
	}

	deployer := deploy.New(br.docker, logger, br.workDir, m.bindMounts)
	newState, err := deployer.Up(ctx, br.spec, br.images, deploy.UpOptions{
		PrevState: prevState,
		Force:     opts.Force,
	})
	if err != nil {
		return err
	}

	if br.workDir != nil && newState != nil {
		if saveErr := deploy.SaveState(br.workDir.Root(), newState); saveErr != nil {
			logger.Warn("failed to save deploy state", "error", saveErr)
		}
	}

	return nil
}

type resolvedProject struct {
	specPath string
	spec     *spec.Network
	deployer *deploy.Deployer
	docker   docker.Client
}

func (m *Manager) resolveAndDeploy(ctx context.Context, name string, logger *slog.Logger) (*resolvedProject, error) {
	specPath, err := m.Resolve(name)
	if err != nil {
		return nil, err
	}

	s, err := spec.Load(specPath)
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	deployer := deploy.New(dockerClient, logger, nil, m.bindMounts)
	return &resolvedProject{specPath: specPath, spec: s, deployer: deployer, docker: dockerClient}, nil
}

func (m *Manager) Down(ctx context.Context, name string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, name, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.Down(ctx, rp.spec)
}

func (m *Manager) Build(ctx context.Context, name string, noCache bool, logger *slog.Logger) error {
	specPath, err := m.Resolve(name)
	if err != nil {
		return err
	}

	br, err := m.doBuild(ctx, specPath, build.Options{NoCache: noCache, ForceBuild: true}, logger)
	if err != nil {
		return err
	}
	_ = br.docker.Close()
	return nil
}

func (m *Manager) Status(ctx context.Context, name string) (*deploy.NetworkStatus, error) {
	rp, err := m.resolveAndDeploy(ctx, name, m.logger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.Status(ctx, rp.spec)
}

func (m *Manager) Prune(ctx context.Context, name string, target PruneTarget, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, name, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	return ExecutePrune(ctx, rp.deployer, rp.spec, filepath.Dir(rp.specPath), target, logger)
}

func ExecutePrune(ctx context.Context, deployer *deploy.Deployer, s *spec.Network, repoRoot string, target PruneTarget, logger *slog.Logger) error {
	switch target {
	case PruneAll:
		var errs []error
		if err := deployer.Down(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("stopping servers: %w", err))
		}
		if err := deployer.PruneImages(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("pruning images: %w", err))
		}
		if err := deployer.PruneVolumes(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("pruning volumes: %w", err))
		}
		if wd, wdErr := build.NewWorkDir(repoRoot, logger); wdErr == nil {
			if cleanErr := wd.Clean(); cleanErr != nil {
				errs = append(errs, fmt.Errorf("cleaning .ore directory: %w", cleanErr))
			}
		}
		if len(errs) == 0 {
			logger.Info("cleaned all resources")
		}
		return errors.Join(errs...)
	case PruneContainers:
		return deployer.Down(ctx, s)
	case PruneImages:
		return deployer.PruneImages(ctx, s)
	case PruneVolumes:
		return deployer.PruneVolumes(ctx, s)
	default:
		return fmt.Errorf("unknown prune target: %d", target)
	}
}

func (m *Manager) Clean(_ context.Context, name string, target CleanTarget, logger *slog.Logger) error {
	specPath, err := m.Resolve(name)
	if err != nil {
		return err
	}

	repoRoot := filepath.Dir(specPath)
	wd, err := build.NewWorkDir(repoRoot, logger)
	if err != nil {
		return fmt.Errorf("opening .ore directory: %w", err)
	}

	switch target {
	case CleanAll:
		return wd.Clean()
	case CleanCache:
		return wd.CleanCache()
	case CleanBuilds:
		return wd.CleanBuilds()
	default:
		return fmt.Errorf("unknown clean target: %d", target)
	}
}

func (m *Manager) Deploy(ctx context.Context, name string, opts UpOptions) error {
	m.logger.Info("deploying project", "project", name)

	if err := m.Pull(ctx, name); err != nil {
		return fmt.Errorf("pulling %s: %w", name, err)
	}

	if err := m.Up(ctx, name, opts, m.logger); err != nil {
		return fmt.Errorf("deploying %s: %w", name, err)
	}

	m.logger.Info("deploy complete", "project", name)
	return nil
}
