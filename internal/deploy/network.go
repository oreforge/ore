package deploy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/network"

	"github.com/oreforge/ore/internal/docker"
)

func EnsureNetwork(ctx context.Context, client docker.Client, name string, logger *slog.Logger) error {
	networks, err := client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == name {
			logger.Debug("network already exists", "network", name)
			return nil
		}
	}

	logger.Info("creating network", "network", name)
	_, err = client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return fmt.Errorf("creating network %s: %w", name, err)
	}

	return nil
}

func RemoveNetwork(ctx context.Context, client docker.Client, name string, logger *slog.Logger) error {
	logger.Info("removing network", "network", name)
	return client.NetworkRemove(ctx, name)
}
