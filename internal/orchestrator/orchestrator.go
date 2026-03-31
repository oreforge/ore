package orchestrator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/cache"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

type Orchestrator struct {
	docker     docker.Client
	logger     *slog.Logger
	workDir    *cache.Manager
	bindMounts bool
}

func New(dockerClient docker.Client, logger *slog.Logger, workDir *cache.Manager, bindMounts bool) *Orchestrator {
	return &Orchestrator{
		docker:     dockerClient,
		logger:     logger,
		workDir:    workDir,
		bindMounts: bindMounts,
	}
}

func (o *Orchestrator) Up(ctx context.Context, cfg *spec.NetworkSpec, images map[string]build.Result) error {
	if err := StopAllOreContainers(ctx, o.docker, cfg.Network, o.logger); err != nil {
		o.logger.Warn("failed to clean orphaned containers", "error", err)
	}

	if err := EnsureNetwork(ctx, o.docker, cfg.Network, o.logger); err != nil {
		return err
	}

	for _, svc := range cfg.Services {
		if err := EnsureImage(ctx, o.docker, svc.Image, o.logger); err != nil {
			return fmt.Errorf("pulling image for %s: %w", svc.Name, err)
		}

		if err := EnsureServiceVolumes(ctx, o.docker, &svc, cfg.Network, o.logger); err != nil {
			return fmt.Errorf("ensuring volumes for %s: %w", svc.Name, err)
		}

		name := ServiceContainerName(&svc)
		if err := StartServiceContainer(ctx, o.docker, &svc, name, cfg.Network, o.logger); err != nil {
			return fmt.Errorf("starting %s: %w", name, err)
		}

		if err := WaitForRunning(ctx, o.docker, name, 10*time.Second); err != nil {
			return fmt.Errorf("service %s failed to start: %w", name, err)
		}

		if err := WaitForHealthy(ctx, o.docker, name, 60*time.Second, o.logger); err != nil {
			o.logger.Warn("service health check failed", "service", name, "error", err)
		}
	}

	for _, srv := range cfg.Servers {
		var res build.Result
		if images != nil {
			var ok bool
			res, ok = images[srv.Name]
			if !ok {
				return fmt.Errorf("no image found for server %s", srv.Name)
			}
		}

		if err := EnsureVolumes(ctx, o.docker, &srv, cfg.Network, o.logger); err != nil {
			return fmt.Errorf("ensuring volumes for %s: %w", srv.Name, err)
		}

		dataBind := o.resolveDataBind(res.ImageTag, srv.Name)
		name := ContainerName(&srv)

		tag := res.ImageTag
		if tag == "" {
			tag = fmt.Sprintf("ore/%s:latest", srv.Name)
		}

		if err := StartContainer(ctx, o.docker, &srv, name, tag, cfg.Network, dataBind, o.logger); err != nil {
			return fmt.Errorf("starting %s: %w", name, err)
		}

		if err := WaitForRunning(ctx, o.docker, name, 10*time.Second); err != nil {
			return fmt.Errorf("container %s failed to start: %w", name, err)
		}

		healthTimeout := res.HealthTimeout
		if healthTimeout == 0 {
			healthTimeout = 3 * time.Minute
		}
		if err := WaitForHealthy(ctx, o.docker, name, healthTimeout, o.logger); err != nil {
			o.logger.Warn("health check failed", "container", name, "error", err)
		}
	}

	return nil
}

func (o *Orchestrator) resolveDataBind(imageTag, serverName string) string {
	if !o.bindMounts || o.workDir == nil || imageTag == "" {
		return ""
	}

	parts := strings.SplitN(imageTag, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	cacheKey := parts[1]

	dataDir := filepath.Join(o.workDir.Root(), "builds", serverName+"-"+cacheKey, "data")

	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return ""
	}

	return abs
}

func (o *Orchestrator) Down(ctx context.Context, cfg *spec.NetworkSpec) error {
	if err := StopAllOreContainers(ctx, o.docker, cfg.Network, o.logger); err != nil {
		o.logger.Warn("failed to stop containers by label", "error", err)
	}

	for i := len(cfg.Servers) - 1; i >= 0; i-- {
		srv := cfg.Servers[i]
		if err := StopContainer(ctx, o.docker, ContainerName(&srv), o.logger); err != nil {
			o.logger.Debug("stopping container by name", "name", srv.Name, "error", err)
		}
	}

	for i := len(cfg.Services) - 1; i >= 0; i-- {
		svc := cfg.Services[i]
		if err := StopContainer(ctx, o.docker, ServiceContainerName(&svc), o.logger); err != nil {
			o.logger.Debug("stopping service container by name", "name", svc.Name, "error", err)
		}
	}

	if err := RemoveNetwork(ctx, o.docker, cfg.Network, o.logger); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			o.logger.Warn("failed to remove network", "network", cfg.Network, "error", err)
		}
	}

	o.logger.Info("down complete", "network", cfg.Network)
	return nil
}

func (o *Orchestrator) PruneImages(ctx context.Context, cfg *spec.NetworkSpec) error {
	images, err := o.docker.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			for _, srv := range cfg.Servers {
				if strings.HasPrefix(tag, "ore/"+srv.Name+":") {
					o.logger.Info("removing image", "tag", tag)
					if _, err := o.docker.ImageRemove(ctx, tag, image.RemoveOptions{Force: true}); err != nil {
						o.logger.Warn("failed to remove image", "tag", tag, "error", err)
					}
				}
			}
		}
	}

	o.logger.Info("pruned images")
	return nil
}

func (o *Orchestrator) PruneVolumes(ctx context.Context, cfg *spec.NetworkSpec) error {
	for _, srv := range cfg.Servers {
		if err := RemoveVolumes(ctx, o.docker, &srv, cfg.Network, o.logger); err != nil {
			o.logger.Warn("failed to remove volumes", "server", srv.Name, "error", err)
		}
	}

	for _, svc := range cfg.Services {
		if err := RemoveServiceVolumes(ctx, o.docker, &svc, cfg.Network, o.logger); err != nil {
			o.logger.Warn("failed to remove volumes", "service", svc.Name, "error", err)
		}
	}

	o.logger.Info("pruned volumes")
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
