package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/software/providers"
	"github.com/oreforge/ore/internal/spec"
)

type CleanTarget int

const (
	CleanAll CleanTarget = iota
	CleanContainers
	CleanImages
	CleanVolumes
	CleanCache
	CleanBuilds
)

type UpOptions struct {
	NoCache bool
	Force   bool
}

func (t CleanTarget) String() string {
	switch t {
	case CleanAll:
		return "all"
	case CleanContainers:
		return "containers"
	case CleanImages:
		return "images"
	case CleanVolumes:
		return "volumes"
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
			logger.Warn("failed to save deploy state", "project", name, "error", saveErr)
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

func (m *Manager) Detail(name string) (specPath string, s *spec.Network, state *deploy.State, err error) {
	specPath, err = m.Resolve(name)
	if err != nil {
		return "", nil, nil, err
	}

	s, err = spec.Load(specPath)
	if err != nil {
		return "", nil, nil, err
	}

	repoRoot := filepath.Dir(specPath)
	if wd, wdErr := build.NewWorkDir(repoRoot, m.logger); wdErr == nil {
		state = deploy.LoadState(wd.Root())
	} else {
		state = deploy.NewDeployState()
	}

	return specPath, s, state, nil
}

func (m *Manager) IconPath(name string) (string, error) {
	specPath, err := m.Resolve(name)
	if err != nil {
		return "", err
	}

	s, err := spec.Load(specPath)
	if err != nil {
		return "", err
	}

	if s.Icon == "" {
		return "", fmt.Errorf("project %q has no icon configured", name)
	}

	projectDir := filepath.Dir(specPath)
	abs, err := filepath.Abs(filepath.Join(projectDir, s.Icon))
	if err != nil {
		return "", fmt.Errorf("resolving icon path: %w", err)
	}

	absProject, _ := filepath.Abs(projectDir)
	if !strings.HasPrefix(abs, absProject+string(filepath.Separator)) {
		return "", fmt.Errorf("icon path escapes project directory")
	}

	return abs, nil
}

func (m *Manager) Builds(name string) (*build.Manifest, error) {
	specPath, err := m.Resolve(name)
	if err != nil {
		return nil, err
	}

	repoRoot := filepath.Dir(specPath)
	wd, err := build.NewWorkDir(repoRoot, m.logger)
	if err != nil {
		return nil, fmt.Errorf("opening .ore directory: %w", err)
	}

	return wd.Manifest(), nil
}

func (m *Manager) Clean(ctx context.Context, name string, target CleanTarget, logger *slog.Logger) error {
	specPath, err := m.Resolve(name)
	if err != nil {
		return err
	}

	repoRoot := filepath.Dir(specPath)
	return ExecuteClean(ctx, specPath, repoRoot, target, m.bindMounts, logger)
}

func ExecuteClean(ctx context.Context, specPath, repoRoot string, target CleanTarget, bindMounts bool, logger *slog.Logger) error {
	needsDocker := target == CleanAll || target == CleanContainers || target == CleanImages || target == CleanVolumes

	var deployer *deploy.Deployer
	var s *spec.Network
	if needsDocker {
		var err error
		s, err = spec.Load(specPath)
		if err != nil {
			return err
		}

		dockerClient, err := docker.New(ctx)
		if err != nil {
			return fmt.Errorf("connecting to Docker: %w", err)
		}
		defer func() { _ = dockerClient.Close() }()

		deployer = deploy.New(dockerClient, logger, nil, bindMounts)
	}

	switch target {
	case CleanAll:
		var errs []error
		if err := deployer.Down(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("stopping containers: %w", err))
		}
		if err := deployer.CleanImages(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("cleaning images: %w", err))
		}
		if err := deployer.CleanVolumes(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("cleaning volumes: %w", err))
		}
		if wd, wdErr := build.NewWorkDir(repoRoot, logger); wdErr == nil {
			if cleanErr := wd.Clean(); cleanErr != nil {
				errs = append(errs, fmt.Errorf("cleaning .ore directory: %w", cleanErr))
			}
		}
		if len(errs) > 0 {
			logger.Warn("clean all completed with errors", "network", s.Network, "errors", errors.Join(errs...))
			return errors.Join(errs...)
		}
		logger.Info("cleaned all resources", "network", s.Network)
		return nil
	case CleanContainers:
		return deployer.Down(ctx, s)
	case CleanImages:
		return deployer.CleanImages(ctx, s)
	case CleanVolumes:
		return deployer.CleanVolumes(ctx, s)
	case CleanCache:
		wd, err := build.NewWorkDir(repoRoot, logger)
		if err != nil {
			return fmt.Errorf("opening .ore directory: %w", err)
		}
		return wd.CleanCache()
	case CleanBuilds:
		wd, err := build.NewWorkDir(repoRoot, logger)
		if err != nil {
			return fmt.Errorf("opening .ore directory: %w", err)
		}
		return wd.CleanBuilds()
	default:
		return fmt.Errorf("unknown clean target: %d", target)
	}
}

func (m *Manager) StartServer(ctx context.Context, projectName, serverName string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, projectName, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	var state *deploy.State
	if wd, wdErr := build.NewWorkDir(filepath.Dir(rp.specPath), logger); wdErr == nil {
		state = deploy.LoadState(wd.Root())
	}

	return rp.deployer.StartServer(ctx, rp.spec, serverName, state, logger)
}

func (m *Manager) StopServer(ctx context.Context, projectName, serverName string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, projectName, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.StopServer(ctx, rp.spec, serverName, logger)
}

func (m *Manager) RestartServer(ctx context.Context, projectName, serverName string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, projectName, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	var state *deploy.State
	if wd, wdErr := build.NewWorkDir(filepath.Dir(rp.specPath), logger); wdErr == nil {
		state = deploy.LoadState(wd.Root())
	}

	return rp.deployer.RestartServer(ctx, rp.spec, serverName, state, logger)
}

func (m *Manager) ServerStatus(ctx context.Context, projectName, serverName string) (*deploy.ServerStatus, error) {
	rp, err := m.resolveAndDeploy(ctx, projectName, m.logger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.ServerStatus(ctx, rp.spec, serverName)
}

func (m *Manager) StartService(ctx context.Context, projectName, serviceName string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, projectName, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.StartService(ctx, rp.spec, serviceName, logger)
}

func (m *Manager) StopService(ctx context.Context, projectName, serviceName string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, projectName, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.StopService(ctx, rp.spec, serviceName, logger)
}

func (m *Manager) RestartService(ctx context.Context, projectName, serviceName string, logger *slog.Logger) error {
	rp, err := m.resolveAndDeploy(ctx, projectName, logger)
	if err != nil {
		return err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.RestartService(ctx, rp.spec, serviceName, logger)
}

func (m *Manager) ServiceStatus(ctx context.Context, projectName, serviceName string) (*deploy.ServerStatus, error) {
	rp, err := m.resolveAndDeploy(ctx, projectName, m.logger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rp.docker.Close() }()

	return rp.deployer.ServiceStatus(ctx, rp.spec, serviceName)
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
