package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/oreforge/ore/internal/spec"
)

type serverRef struct {
	server  *spec.Server
	service *spec.Service
}

func findServer(cfg *spec.Network, name string) (serverRef, error) {
	for i := range cfg.Servers {
		if cfg.Servers[i].Name == name {
			return serverRef{server: &cfg.Servers[i]}, nil
		}
	}
	for i := range cfg.Services {
		if cfg.Services[i].Name == name {
			return serverRef{service: &cfg.Services[i]}, nil
		}
	}
	return serverRef{}, fmt.Errorf("server %q not found in network %q", name, cfg.Network)
}

func (d *Deployer) StartServer(ctx context.Context, cfg *spec.Network, serverName string, state *State, logger *slog.Logger) error {
	ref, err := findServer(cfg, serverName)
	if err != nil {
		return err
	}

	if err := EnsureNetwork(ctx, d.docker, cfg.Network, logger); err != nil {
		return err
	}

	if ref.service != nil {
		return d.startService(ctx, cfg, ref.service, logger)
	}
	return d.startServer(ctx, cfg, ref.server, state, logger)
}

func (d *Deployer) startServer(ctx context.Context, cfg *spec.Network, srv *spec.Server, state *State, logger *slog.Logger) error {
	tag := ""
	if state != nil {
		tag = state.Servers[srv.Name].ImageTag
	}
	if tag == "" {
		info, err := d.docker.ContainerInspect(ctx, srv.Name)
		if err == nil {
			tag = info.Config.Image
		}
	}
	if tag == "" {
		return fmt.Errorf("server %q has not been deployed yet — run up first", srv.Name)
	}

	if err := EnsureVolumes(ctx, d.docker, srv, cfg.Network, logger); err != nil {
		return err
	}

	dataBind := d.resolveDataBind(tag, srv.Name)
	if err := StartContainer(ctx, d.docker, srv, srv.Name, tag, cfg.Network, dataBind, logger); err != nil {
		return err
	}

	logger.Info("waiting for container", "server", srv.Name)
	if err := WaitForRunning(ctx, d.docker, srv.Name, 10*time.Second); err != nil {
		return err
	}

	healthTimeout := 3 * time.Minute
	if err := WaitForHealthy(ctx, d.docker, srv.Name, healthTimeout, logger); err != nil {
		logger.Warn("health check failed", "server", srv.Name, "error", err)
	}

	return nil
}

func (d *Deployer) startService(ctx context.Context, cfg *spec.Network, svc *spec.Service, logger *slog.Logger) error {
	info, err := d.docker.ContainerInspect(ctx, svc.Name)
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting service %s: %w", svc.Name, err)
	}

	if cerrdefs.IsNotFound(err) || info.Config == nil {
		if err := EnsureImage(ctx, d.docker, svc.Image, logger); err != nil {
			return err
		}
	}

	if err := EnsureServiceVolumes(ctx, d.docker, svc, cfg.Network, logger); err != nil {
		return err
	}

	svcHC := resolveServiceHealthCheck(svc.HealthCheck)
	if err := StartServiceContainer(ctx, d.docker, svc, svc.Name, cfg.Network, svcHC, logger); err != nil {
		return err
	}

	logger.Info("waiting for service", "service", svc.Name)
	if err := WaitForRunning(ctx, d.docker, svc.Name, 10*time.Second); err != nil {
		return err
	}

	if hcTimeout := svcHC.WaitTimeout(); hcTimeout > 0 {
		if err := WaitForHealthy(ctx, d.docker, svc.Name, hcTimeout, logger); err != nil {
			logger.Warn("service health check failed", "service", svc.Name, "error", err)
		}
	}

	return nil
}

func (d *Deployer) StopServer(ctx context.Context, cfg *spec.Network, serverName string, logger *slog.Logger) error {
	if _, err := findServer(cfg, serverName); err != nil {
		return err
	}
	return StopContainer(ctx, d.docker, serverName, logger)
}

func (d *Deployer) RestartServer(ctx context.Context, cfg *spec.Network, serverName string, state *State, logger *slog.Logger) error {
	ref, err := findServer(cfg, serverName)
	if err != nil {
		return err
	}

	logger.Info("stopping server", "server", serverName)
	if err := StopContainer(ctx, d.docker, serverName, logger); err != nil {
		return fmt.Errorf("stopping %s: %w", serverName, err)
	}

	if err := EnsureNetwork(ctx, d.docker, cfg.Network, logger); err != nil {
		return err
	}

	if ref.service != nil {
		return d.startService(ctx, cfg, ref.service, logger)
	}
	return d.startServer(ctx, cfg, ref.server, state, logger)
}

func (d *Deployer) ServerStatus(ctx context.Context, cfg *spec.Network, serverName string) (*ServerStatus, error) {
	if _, err := findServer(cfg, serverName); err != nil {
		return nil, err
	}

	cs := d.inspectContainer(ctx, serverName)
	return &ServerStatus{
		Name:      serverName,
		Container: cs,
	}, nil
}
