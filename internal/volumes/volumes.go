package volumes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockervolume "github.com/docker/docker/api/types/volume"

	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
)

const SizeUnknown int64 = -1

type Volume struct {
	Name       string            `json:"name"`
	Project    string            `json:"project"`
	Owner      string            `json:"owner"`
	OwnerKind  string            `json:"ownerKind"`
	Logical    string            `json:"logical"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Labels     map[string]string `json:"labels,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
	SizeBytes  int64             `json:"sizeBytes"`
	InUseBy    []string          `json:"inUseBy"`
}

var ErrNotFound = errors.New("volume not found")

type Service struct {
	docker docker.Client
	log    *slog.Logger
}

func New(d docker.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{docker: d, log: logger}
}

func (s *Service) List(ctx context.Context, project string) ([]Volume, error) {
	args := filters.NewArgs()
	args.Add("label", deploy.LabelManaged+"=true")
	if project != "" {
		args.Add("label", deploy.LabelProject+"="+project)
	}

	resp, err := s.docker.VolumeList(ctx, dockervolume.ListOptions{Filters: args})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	usage, err := s.buildUsageIndex(ctx, project)
	if err != nil {
		s.log.Warn("failed to build volume usage index", "error", err)
		usage = map[string][]string{}
	}

	out := make([]Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		if v == nil {
			continue
		}
		out = append(out, toVolume(v, usage[v.Name]))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Service) Inspect(ctx context.Context, name string) (Volume, error) {
	v, err := s.docker.VolumeInspect(ctx, name)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return Volume{}, ErrNotFound
		}
		return Volume{}, fmt.Errorf("inspecting volume %s: %w", name, err)
	}

	if v.Labels[deploy.LabelManaged] != "true" {
		return Volume{}, ErrNotFound
	}

	inUseBy, err := s.inUseBy(ctx, v.Name)
	if err != nil {
		s.log.Warn("failed to resolve volume users", "volume", name, "error", err)
	}

	return toVolume(&v, inUseBy), nil
}

func (s *Service) inUseBy(ctx context.Context, volumeName string) ([]string, error) {
	args := filters.NewArgs()
	args.Add("volume", volumeName)

	containers, err := s.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, fmt.Errorf("listing containers for volume %s: %w", volumeName, err)
	}

	out := make([]string, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		out = append(out, containerDisplayName(name, c.ID))
	}
	return out, nil
}

func (s *Service) buildUsageIndex(ctx context.Context, project string) (map[string][]string, error) {
	args := filters.NewArgs()
	args.Add("label", deploy.LabelManaged+"=true")
	if project != "" {
		args.Add("label", deploy.LabelProject+"="+project)
	}

	containers, err := s.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	usage := make(map[string][]string, len(containers))
	for _, c := range containers {
		info, inspectErr := s.docker.ContainerInspect(ctx, c.ID)
		if inspectErr != nil {
			s.log.Debug("skipping container during usage scan", "id", c.ID, "error", inspectErr)
			continue
		}
		displayName := containerDisplayName(info.Name, c.ID)
		for _, m := range info.Mounts {
			if m.Type != "volume" || m.Name == "" {
				continue
			}
			usage[m.Name] = append(usage[m.Name], displayName)
		}
	}
	return usage, nil
}

func toVolume(v *dockervolume.Volume, inUseBy []string) Volume {
	labels := v.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	created := parseCreatedAt(labels[deploy.LabelCreatedAt], v.CreatedAt)
	return Volume{
		Name:       v.Name,
		Project:    labels[deploy.LabelProject],
		Owner:      labels[deploy.LabelOwner],
		OwnerKind:  labels[deploy.LabelOwnerKind],
		Logical:    labels[deploy.LabelVolume],
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     labels,
		CreatedAt:  created,
		SizeBytes:  SizeUnknown,
		InUseBy:    append([]string(nil), inUseBy...),
	}
}

func parseCreatedAt(label, dockerCreated string) time.Time {
	for _, raw := range []string{label, dockerCreated} {
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func containerDisplayName(name, id string) string {
	if len(name) > 1 && name[0] == '/' {
		return name[1:]
	}
	if name != "" {
		return name
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
