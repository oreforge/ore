package orchestrator

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

func EnsureVolumes(ctx context.Context, client docker.Client, srv *spec.ServerSpec, networkName string, logger *slog.Logger) error {
	for _, containerName := range ContainerNames(srv) {
		for _, vol := range srv.Volumes {
			name := volumeName(networkName, containerName, vol.Name)
			logger.Debug("ensuring volume", "volume", name)
			_, err := client.VolumeCreate(ctx, volume.CreateOptions{Name: name})
			if err != nil {
				return fmt.Errorf("creating volume %s: %w", name, err)
			}
		}
	}
	return nil
}

func RemoveVolumes(ctx context.Context, client docker.Client, srv *spec.ServerSpec, networkName string, logger *slog.Logger) error {
	for _, containerName := range ContainerNames(srv) {
		for _, vol := range srv.Volumes {
			name := volumeName(networkName, containerName, vol.Name)
			logger.Info("removing volume", "volume", name)
			if err := client.VolumeRemove(ctx, name, true); err != nil {
				return fmt.Errorf("removing volume %s: %w", name, err)
			}
		}
	}
	return nil
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
