package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/volume"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

const (
	LabelManaged   = "ore.managed"
	LabelNetwork   = "ore.network"
	LabelServer    = "ore.server"
	LabelProject   = "ore.project"
	LabelOwner     = "ore.owner"
	LabelOwnerKind = "ore.owner.kind"
	LabelVolume    = "ore.volume"
	LabelCreatedAt = "ore.created.at"

	OwnerKindServer  = "server"
	OwnerKindService = "service"
)

func VolumeNameFor(networkName, ownerContainer, logical string) string {
	return networkName + "_" + ownerContainer + "_" + logical
}

func DeclaredVolumeNames(s *spec.Network) map[string]struct{} {
	out := make(map[string]struct{})
	for i := range s.Servers {
		srv := s.Servers[i]
		for _, v := range srv.Volumes {
			out[VolumeNameFor(s.Network, ContainerName(&srv), v.Name)] = struct{}{}
		}
	}
	for i := range s.Services {
		svc := s.Services[i]
		for _, v := range svc.Volumes {
			out[VolumeNameFor(s.Network, ServiceContainerName(&svc), v.Name)] = struct{}{}
		}
	}
	return out
}

func managedLabels(networkName, owner, ownerKind, logical string) map[string]string {
	return map[string]string{
		LabelManaged:   "true",
		LabelProject:   networkName,
		LabelOwner:     owner,
		LabelOwnerKind: ownerKind,
		LabelVolume:    logical,
		LabelCreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func ensureVolumes(ctx context.Context, client docker.Client, ownerName, ownerKind, networkName string, vols []spec.Volume, logger *slog.Logger) error {
	for _, vol := range vols {
		name := VolumeNameFor(networkName, ownerName, vol.Name)
		logger.Debug("ensuring volume", "volume", name)
		opts := volume.CreateOptions{
			Name:   name,
			Labels: managedLabels(networkName, ownerName, ownerKind, vol.Name),
		}
		if _, err := client.VolumeCreate(ctx, opts); err != nil {
			return fmt.Errorf("creating volume %s: %w", name, err)
		}
	}
	return nil
}

func EnsureVolumes(ctx context.Context, client docker.Client, srv *spec.Server, networkName string, logger *slog.Logger) error {
	return ensureVolumes(ctx, client, ContainerName(srv), OwnerKindServer, networkName, srv.Volumes, logger)
}

func EnsureServiceVolumes(ctx context.Context, client docker.Client, svc *spec.Service, networkName string, logger *slog.Logger) error {
	return ensureVolumes(ctx, client, ServiceContainerName(svc), OwnerKindService, networkName, svc.Volumes, logger)
}

func removeVolumes(ctx context.Context, client docker.Client, containerName, networkName string, vols []spec.Volume, logger *slog.Logger) error {
	for _, vol := range vols {
		name := VolumeNameFor(networkName, containerName, vol.Name)
		logger.Debug("removing volume", "volume", name)
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
