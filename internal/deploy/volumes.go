package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/volume"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

func volumeName(networkName, containerName, volName string) string {
	return networkName + "_" + containerName + "_" + volName
}

func ensureVolumes(ctx context.Context, client docker.Client, containerName, networkName string, vols []spec.Volume, logger *slog.Logger) error {
	for _, vol := range vols {
		name := volumeName(networkName, containerName, vol.Name)
		logger.Debug("ensuring volume", "volume", name)
		if _, err := client.VolumeCreate(ctx, volume.CreateOptions{Name: name}); err != nil {
			return fmt.Errorf("creating volume %s: %w", name, err)
		}
	}
	return nil
}

func EnsureVolumes(ctx context.Context, client docker.Client, srv *spec.Server, networkName string, logger *slog.Logger) error {
	return ensureVolumes(ctx, client, ContainerName(srv), networkName, srv.Volumes, logger)
}

func EnsureServiceVolumes(ctx context.Context, client docker.Client, svc *spec.Service, networkName string, logger *slog.Logger) error {
	return ensureVolumes(ctx, client, ServiceContainerName(svc), networkName, svc.Volumes, logger)
}

func removeVolumes(ctx context.Context, client docker.Client, containerName, networkName string, vols []spec.Volume, logger *slog.Logger) error {
	for _, vol := range vols {
		name := volumeName(networkName, containerName, vol.Name)
		logger.Info("removing volume", "volume", name)
		if err := client.VolumeRemove(ctx, name, true); err != nil {
			return fmt.Errorf("removing volume %s: %w", name, err)
		}
	}
	return nil
}

func RemoveVolumes(ctx context.Context, client docker.Client, srv *spec.Server, networkName string, logger *slog.Logger) error {
	return removeVolumes(ctx, client, ContainerName(srv), networkName, srv.Volumes, logger)
}

func RemoveServiceVolumes(ctx context.Context, client docker.Client, svc *spec.Service, networkName string, logger *slog.Logger) error {
	return removeVolumes(ctx, client, ServiceContainerName(svc), networkName, svc.Volumes, logger)
}

func parseMemory(mem string) (int64, error) {
	mem = strings.TrimSpace(mem)
	if len(mem) < 2 {
		return 0, fmt.Errorf("invalid memory format: %s", mem)
	}

	suffix := mem[len(mem)-1]
	numStr := mem[:len(mem)-1]
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value: %s", mem)
	}

	switch suffix {
	case 'M', 'm':
		return int64(num * 1024 * 1024), nil
	case 'G', 'g':
		return int64(num * 1024 * 1024 * 1024), nil
	default:
		return 0, fmt.Errorf("unknown memory suffix %c in %s (use M or G)", suffix, mem)
	}
}

func parseCPU(cpu string) (int64, error) {
	val, err := strconv.ParseFloat(cpu, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpu value: %s", cpu)
	}
	return int64(val * 1e9), nil
}
