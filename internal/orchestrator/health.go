package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/oreforge/ore/internal/docker"
)

func WaitForHealthy(ctx context.Context, client docker.Client, containerName string, timeout time.Duration, logger *slog.Logger) error {
	deadline := time.Now().Add(timeout)

	logger.Debug("waiting for container health", "container", containerName, "timeout", timeout)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("health check timed out for %s after %s", containerName, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := client.ContainerInspect(ctx, containerName)
			if err != nil {
				logger.Debug("inspect failed during health check", "container", containerName, "error", err)
				continue
			}

			if info.State.Status == "exited" || info.State.Status == "dead" {
				return fmt.Errorf("container %s is %s (exit code %d)", containerName, info.State.Status, info.State.ExitCode)
			}

			if info.State.Health != nil {
				switch info.State.Health.Status {
				case "healthy":
					logger.Info("health check passed", "container", containerName)
					return nil
				case "unhealthy":
					return fmt.Errorf("container %s is unhealthy", containerName)
				default:
					logger.Debug("container health starting", "container", containerName, "status", info.State.Health.Status)
				}
			} else if info.State.Running {
				logger.Info("container running (no healthcheck)", "container", containerName)
				return nil
			}
		}
	}
}
