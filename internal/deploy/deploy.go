package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"golang.org/x/sync/errgroup"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

type Deployer struct {
	docker     docker.Client
	logger     *slog.Logger
	workDir    *build.WorkDir
	bindMounts bool
}

func New(dockerClient docker.Client, logger *slog.Logger, workDir *build.WorkDir, bindMounts bool) *Deployer {
	return &Deployer{
		docker:     dockerClient,
		logger:     logger,
		workDir:    workDir,
		bindMounts: bindMounts,
	}
}

type UpOptions struct {
	PrevState *State
	Force     bool
}

func (o UpOptions) prevServer(name string) ServerState {
	if o.PrevState == nil {
		return ServerState{}
	}
	return o.PrevState.Servers[name]
}

func (o UpOptions) prevService(name string) ServiceState {
	if o.PrevState == nil {
		return ServiceState{}
	}
	return o.PrevState.Services[name]
}

func (d *Deployer) Up(ctx context.Context, cfg *spec.Network, images map[string]build.Result, opts UpOptions) (*State, error) {
	if opts.Force || opts.PrevState == nil {
		if err := StopAllOreContainers(ctx, d.docker, cfg.Network, d.logger); err != nil {
			d.logger.Warn("failed to clean orphaned containers", "network", cfg.Network, "error", err)
		}
	} else {
		d.stopRemovedContainers(ctx, cfg, opts.PrevState)
	}

	if err := EnsureNetwork(ctx, d.docker, cfg.Network, d.logger); err != nil {
		return nil, err
	}

	newState := NewDeployState()

	serviceByName := make(map[string]*spec.Service, len(cfg.Services))
	for i := range cfg.Services {
		serviceByName[cfg.Services[i].Name] = &cfg.Services[i]
	}
	serverByName := make(map[string]*spec.Server, len(cfg.Servers))
	for i := range cfg.Servers {
		serverByName[cfg.Servers[i].Name] = &cfg.Servers[i]
	}

	ready := make(map[string]chan struct{}, len(cfg.Servers)+len(cfg.Services))
	for _, srv := range cfg.Servers {
		ready[srv.Name] = make(chan struct{})
	}
	for _, svc := range cfg.Services {
		ready[svc.Name] = make(chan struct{})
	}

	var mu sync.Mutex

	groups := spec.TopologicalOrder(cfg)

	for _, group := range groups {
		g, gCtx := errgroup.WithContext(ctx)

		for _, svcName := range group.Services {
			svc := serviceByName[svcName]
			g.Go(func() error {
				if err := waitForDeps(gCtx, svc.DependsOn, ready); err != nil {
					return fmt.Errorf("service %s: dependency wait: %w", svc.Name, err)
				}

				configHash := spec.ServiceHash(svc)
				name := ServiceContainerName(svc)

				prev := opts.prevService(svc.Name)
				if d.unchanged(gCtx, name, svc.Image, prev.Image, configHash, prev.ConfigHash, opts) {
					d.logger.Debug("service unchanged, skipping", "service", name)
					mu.Lock()
					newState.Services[svc.Name] = ServiceState{Image: svc.Image, ConfigHash: configHash}
					mu.Unlock()
					close(ready[svc.Name])
					return nil
				}

				if err := EnsureImage(gCtx, d.docker, svc.Image, d.logger); err != nil {
					return fmt.Errorf("pulling image for %s: %w", svc.Name, err)
				}

				if err := EnsureServiceVolumes(gCtx, d.docker, svc, cfg.Network, d.logger); err != nil {
					return fmt.Errorf("ensuring volumes for %s: %w", svc.Name, err)
				}

				svcHC := resolveServiceHealthCheck(svc.HealthCheck)

				if err := StartServiceContainer(gCtx, d.docker, svc, name, cfg.Network, svcHC, d.logger); err != nil {
					return fmt.Errorf("starting %s: %w", name, err)
				}

				if err := WaitForRunning(gCtx, d.docker, name, 10*time.Second); err != nil {
					return fmt.Errorf("service %s failed to start: %w", name, err)
				}

				if hcTimeout := svcHC.WaitTimeout(); hcTimeout > 0 {
					if err := WaitForHealthy(gCtx, d.docker, name, hcTimeout, d.logger); err != nil {
						d.logger.Warn("service health check failed", "service", name, "error", err)
					}
				}

				mu.Lock()
				newState.Services[svc.Name] = ServiceState{Image: svc.Image, ConfigHash: configHash}
				mu.Unlock()
				close(ready[svc.Name])
				return nil
			})
		}

		for _, srvName := range group.Servers {
			srv := serverByName[srvName]
			g.Go(func() error {
				if err := waitForDeps(gCtx, srv.DependsOn, ready); err != nil {
					return fmt.Errorf("server %s: dependency wait: %w", srv.Name, err)
				}

				var res build.Result
				if images != nil {
					var ok bool
					res, ok = images[srv.Name]
					if !ok {
						return fmt.Errorf("no image found for server %s", srv.Name)
					}
				}

				tag := res.ImageTag
				if tag == "" {
					return fmt.Errorf("no image tag for server %s", srv.Name)
				}

				configHash := spec.ServerHash(srv, tag)
				name := ContainerName(srv)

				prev := opts.prevServer(srv.Name)
				if d.unchanged(gCtx, name, tag, prev.ImageTag, configHash, prev.ConfigHash, opts) {
					d.logger.Debug("server unchanged, skipping", "server", name)
					mu.Lock()
					newState.Servers[srv.Name] = ServerState{ImageTag: tag, ConfigHash: configHash}
					mu.Unlock()
					close(ready[srv.Name])
					return nil
				}

				healthTimeout := res.HealthTimeout
				if healthTimeout == 0 {
					healthTimeout = 3 * time.Minute
				}

				if err := EnsureVolumes(gCtx, d.docker, srv, cfg.Network, d.logger); err != nil {
					return fmt.Errorf("ensuring volumes for %s: %w", srv.Name, err)
				}

				dataBind := d.resolveDataBind(tag, srv.Name)

				if err := StartContainer(gCtx, d.docker, srv, name, tag, cfg.Network, dataBind, d.logger); err != nil {
					return fmt.Errorf("starting %s: %w", name, err)
				}

				if err := WaitForRunning(gCtx, d.docker, name, 10*time.Second); err != nil {
					return fmt.Errorf("container %s failed to start: %w", name, err)
				}

				if err := WaitForHealthy(gCtx, d.docker, name, healthTimeout, d.logger); err != nil {
					d.logger.Warn("health check failed", "container", name, "error", err)
				}

				mu.Lock()
				newState.Servers[srv.Name] = ServerState{ImageTag: tag, ConfigHash: configHash}
				mu.Unlock()
				close(ready[srv.Name])
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}
	}

	return newState, nil
}

func waitForDeps(ctx context.Context, deps []spec.Dependency, ready map[string]chan struct{}) error {
	for _, dep := range deps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready[dep.Name]:
		}
	}
	return nil
}

func (d *Deployer) unchanged(ctx context.Context, containerName, expectedImage, prevImage, configHash, prevConfigHash string, opts UpOptions) bool {
	if opts.Force || opts.PrevState == nil {
		return false
	}
	if prevImage != expectedImage || prevConfigHash != configHash {
		return false
	}
	info, err := d.docker.ContainerInspect(ctx, containerName)
	if err != nil {
		return false
	}
	return info.State.Running && info.Config.Image == expectedImage
}

func (d *Deployer) stopRemovedContainers(ctx context.Context, cfg *spec.Network, prev *State) {
	current := make(map[string]bool, len(cfg.Servers)+len(cfg.Services))
	for _, srv := range cfg.Servers {
		current[srv.Name] = true
	}
	for _, svc := range cfg.Services {
		current[svc.Name] = true
	}

	for name := range prev.Servers {
		if !current[name] {
			d.logger.Info("removing server no longer in spec", "server", name)
			if err := stopAndRemove(ctx, d.docker, name, d.logger); err != nil {
				d.logger.Warn("failed to remove orphaned server", "server", name, "error", err)
			}
		}
	}
	for name := range prev.Services {
		if !current[name] {
			d.logger.Info("removing service no longer in spec", "service", name)
			if err := stopAndRemove(ctx, d.docker, name, d.logger); err != nil {
				d.logger.Warn("failed to remove orphaned service", "service", name, "error", err)
			}
		}
	}
}

func (d *Deployer) resolveDataBind(imageTag, serverName string) string {
	if !d.bindMounts || d.workDir == nil || imageTag == "" {
		return ""
	}

	parts := strings.SplitN(imageTag, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	cacheKey := parts[1]

	dataDir := filepath.Join(d.workDir.Root(), "builds", serverName+"-"+cacheKey, "data")

	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return ""
	}

	return abs
}

func (d *Deployer) Down(ctx context.Context, cfg *spec.Network) error {
	if err := StopAllOreContainers(ctx, d.docker, cfg.Network, d.logger); err != nil {
		d.logger.Warn("failed to stop containers by label", "network", cfg.Network, "error", err)
	}

	downG, downCtx := errgroup.WithContext(ctx)
	for _, srv := range cfg.Servers {
		name := ContainerName(&srv)
		downG.Go(func() error {
			if err := StopContainer(downCtx, d.docker, name, d.logger); err != nil {
				d.logger.Debug("stopping container by name", "name", name, "error", err)
			}
			return nil
		})
	}
	for _, svc := range cfg.Services {
		name := ServiceContainerName(&svc)
		downG.Go(func() error {
			if err := StopContainer(downCtx, d.docker, name, d.logger); err != nil {
				d.logger.Debug("stopping service container by name", "name", name, "error", err)
			}
			return nil
		})
	}
	_ = downG.Wait()

	if err := RemoveNetwork(ctx, d.docker, cfg.Network, d.logger); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			d.logger.Warn("failed to remove network", "network", cfg.Network, "error", err)
		}
	}

	d.logger.Info("down complete", "network", cfg.Network)
	return nil
}

func (d *Deployer) PruneImages(ctx context.Context, cfg *spec.Network) error {
	images, err := d.docker.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			for _, srv := range cfg.Servers {
				if strings.HasPrefix(tag, "ore/"+srv.Name+":") {
					d.logger.Debug("removing image", "tag", tag)
					if _, err := d.docker.ImageRemove(ctx, tag, image.RemoveOptions{Force: true}); err != nil {
						d.logger.Warn("failed to remove image", "tag", tag, "error", err)
					}
				}
			}
		}
	}

	for _, img := range images {
		if len(img.RepoTags) == 0 && len(img.RepoDigests) == 0 {
			shortID := img.ID[:min(19, len(img.ID))]
			d.logger.Debug("removing dangling image", "id", shortID)
			if _, err := d.docker.ImageRemove(ctx, img.ID, image.RemoveOptions{}); err != nil {
				d.logger.Debug("failed to remove dangling image", "id", shortID, "error", err)
			}
		}
	}

	d.logger.Info("pruned images", "network", cfg.Network)
	return nil
}

func (d *Deployer) PruneVolumes(ctx context.Context, cfg *spec.Network) error {
	for _, srv := range cfg.Servers {
		if err := RemoveVolumes(ctx, d.docker, &srv, cfg.Network, d.logger); err != nil {
			d.logger.Warn("failed to remove volumes", "server", srv.Name, "error", err)
		}
	}

	for _, svc := range cfg.Services {
		if err := RemoveServiceVolumes(ctx, d.docker, &svc, cfg.Network, d.logger); err != nil {
			d.logger.Warn("failed to remove volumes", "service", svc.Name, "error", err)
		}
	}

	d.logger.Info("pruned volumes", "network", cfg.Network)
	return nil
}

func EnsureImage(ctx context.Context, client docker.Client, imageRef string, logger *slog.Logger) error {
	images, err := client.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", imageRef)),
	})
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	if len(images) > 0 {
		logger.Debug("image already present", "image", imageRef)
		return nil
	}

	logger.Info("pulling image", "image", imageRef)
	reader, err := client.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", imageRef, err)
	}
	defer func() { _ = reader.Close() }()

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("reading pull response for %s: %w", imageRef, err)
	}

	logger.Info("image pulled", "image", imageRef)
	return nil
}
