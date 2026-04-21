package volumes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
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

func (s *Service) List(ctx context.Context, networkName string) ([]Volume, error) {
	args := filters.NewArgs()
	args.Add("label", deploy.LabelManaged+"=true")
	if networkName != "" {
		args.Add("label", deploy.LabelProject+"="+networkName)
	}

	resp, err := s.docker.VolumeList(ctx, dockervolume.ListOptions{Filters: args})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	usage, err := s.buildUsageIndex(ctx, networkName)
	if err != nil {
		s.log.Warn("failed to build volume usage index", "error", err)
		usage = map[string][]string{}
	}

	sizes, err := s.sizeIndex(ctx)
	if err != nil {
		s.log.Warn("failed to read volume disk usage", "error", err)
		sizes = map[string]int64{}
	}

	out := make([]Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		if v == nil {
			continue
		}
		out = append(out, toVolume(v, usage[v.Name], sizeOf(sizes, v.Name)))
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

	size := SizeUnknown
	if sizes, err := s.sizeIndex(ctx); err != nil {
		s.log.Warn("failed to read volume disk usage", "volume", name, "error", err)
	} else {
		size = sizeOf(sizes, v.Name)
	}

	return toVolume(&v, inUseBy, size), nil
}

func (s *Service) sizeIndex(ctx context.Context) (map[string]int64, error) {
	du, err := s.docker.DiskUsage(ctx, types.DiskUsageOptions{
		Types: []types.DiskUsageObject{types.VolumeObject},
	})
	if err != nil {
		return nil, fmt.Errorf("disk usage: %w", err)
	}
	out := make(map[string]int64, len(du.Volumes))
	for _, v := range du.Volumes {
		if v == nil || v.UsageData == nil {
			continue
		}
		out[v.Name] = v.UsageData.Size
	}
	return out, nil
}

func sizeOf(sizes map[string]int64, name string) int64 {
	if size, ok := sizes[name]; ok {
		return size
	}
	return SizeUnknown
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

func (s *Service) buildUsageIndex(ctx context.Context, networkName string) (map[string][]string, error) {
	args := filters.NewArgs()
	args.Add("label", deploy.LabelManaged+"=true")
	if networkName != "" {
		args.Add("label", deploy.LabelNetwork+"="+networkName)
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

func toVolume(v *dockervolume.Volume, inUseBy []string, size int64) Volume {
	labels := v.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return Volume{
		Name:       v.Name,
		Project:    labels[deploy.LabelProject],
		Owner:      labels[deploy.LabelOwner],
		OwnerKind:  labels[deploy.LabelOwnerKind],
		Logical:    labels[deploy.LabelVolume],
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     labels,
		CreatedAt:  parseCreatedAt(labels[deploy.LabelCreatedAt], v.CreatedAt),
		SizeBytes:  size,
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

var ErrInUse = errors.New("volume is in use")

type PruneReport struct {
	Project    string           `json:"project"`
	DryRun     bool             `json:"dryRun"`
	Candidates []PruneCandidate `json:"candidates"`
	Deleted    []string         `json:"deleted,omitempty"`
	Skipped    []PruneSkip      `json:"skipped,omitempty"`
}

type PruneCandidate struct {
	Name    string `json:"name"`
	Owner   string `json:"owner,omitempty"`
	Logical string `json:"logical,omitempty"`
}

type PruneSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (s *Service) Prune(ctx context.Context, networkName string, declared map[string]struct{}, dryRun bool) (*PruneReport, error) {
	managed, err := s.List(ctx, networkName)
	if err != nil {
		return nil, err
	}

	report := &PruneReport{Project: networkName, DryRun: dryRun}
	for _, v := range managed {
		if _, kept := declared[v.Name]; kept {
			continue
		}
		report.Candidates = append(report.Candidates, PruneCandidate{
			Name:    v.Name,
			Owner:   v.Owner,
			Logical: v.Logical,
		})
	}

	if dryRun {
		return report, nil
	}

	for _, c := range report.Candidates {
		if err := s.Remove(ctx, c.Name, false); err != nil {
			if errors.Is(err, ErrInUse) {
				report.Skipped = append(report.Skipped, PruneSkip{Name: c.Name, Reason: "in use"})
				continue
			}
			report.Skipped = append(report.Skipped, PruneSkip{Name: c.Name, Reason: err.Error()})
			continue
		}
		report.Deleted = append(report.Deleted, c.Name)
	}
	return report, nil
}

func (s *Service) Remove(ctx context.Context, volumeName string, force bool) error {
	v, err := s.Inspect(ctx, volumeName)
	if err != nil {
		return err
	}

	if len(v.InUseBy) > 0 {
		if !force {
			return fmt.Errorf("%w: %s", ErrInUse, strings.Join(v.InUseBy, ", "))
		}
		s.log.Info("force-stopping containers to delete volume", "volume", v.Name, "containers", v.InUseBy)
		for _, name := range v.InUseBy {
			if err := deploy.StopContainer(ctx, s.docker, name, s.log); err != nil {
				return fmt.Errorf("stopping container %s: %w", name, err)
			}
		}
	}

	if err := s.docker.VolumeRemove(ctx, v.Name, force); err != nil {
		if cerrdefs.IsNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("removing volume %s: %w", v.Name, err)
	}
	s.log.Info("volume removed", "volume", v.Name)
	return nil
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
