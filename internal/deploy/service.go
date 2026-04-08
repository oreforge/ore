package deploy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/oreforge/ore/internal/spec"
)

func findService(cfg *spec.Network, name string) (*spec.Service, error) {
	for i := range cfg.Services {
		if cfg.Services[i].Name == name {
			return &cfg.Services[i], nil
		}
	}
	return nil, fmt.Errorf("service %q not found in network %q", name, cfg.Network)
}

func (d *Deployer) StartService(ctx context.Context, cfg *spec.Network, serviceName string, logger *slog.Logger) error {
	svc, err := findService(cfg, serviceName)
	if err != nil {
		return err
	}

	if err := EnsureNetwork(ctx, d.docker, cfg.Network, logger); err != nil {
		return err
	}

	return d.startService(ctx, cfg, svc, logger)
}

func (d *Deployer) StopService(ctx context.Context, cfg *spec.Network, serviceName string, logger *slog.Logger) error {
	if _, err := findService(cfg, serviceName); err != nil {
		return err
	}
	return StopContainer(ctx, d.docker, serviceName, logger)
}

func (d *Deployer) RestartService(ctx context.Context, cfg *spec.Network, serviceName string, logger *slog.Logger) error {
	svc, err := findService(cfg, serviceName)
	if err != nil {
		return err
	}

	logger.Info("stopping service", "service", serviceName)
	if err := StopContainer(ctx, d.docker, serviceName, logger); err != nil {
		return fmt.Errorf("stopping %s: %w", serviceName, err)
	}

	if err := EnsureNetwork(ctx, d.docker, cfg.Network, logger); err != nil {
		return err
	}

	return d.startService(ctx, cfg, svc, logger)
}

func (d *Deployer) ServiceStatus(ctx context.Context, cfg *spec.Network, serviceName string) (*ServerStatus, error) {
	if _, err := findService(cfg, serviceName); err != nil {
		return nil, err
	}

	cs := d.inspectContainer(ctx, serviceName)
	return &ServerStatus{
		Name:      serviceName,
		Container: cs,
	}, nil
}
