package volumes

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	dockerbuild "github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockervolume "github.com/docker/docker/api/types/volume"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oreforge/ore/internal/deploy"
)

type fakeDocker struct {
	volumes    []*dockervolume.Volume
	inspect    map[string]dockervolume.Volume
	inspectErr map[string]error
	containers []container.Summary
	mounts     map[string][]container.MountPoint
	names      map[string]string
	listErr    error
	listCalls  int
}

func (f *fakeDocker) VolumeList(ctx context.Context, opts dockervolume.ListOptions) (dockervolume.ListResponse, error) {
	f.listCalls++
	if f.listErr != nil {
		return dockervolume.ListResponse{}, f.listErr
	}
	out := make([]*dockervolume.Volume, 0, len(f.volumes))
	for _, v := range f.volumes {
		if matchesFilters(v.Labels, opts.Filters) {
			out = append(out, v)
		}
	}
	return dockervolume.ListResponse{Volumes: out}, nil
}

func (f *fakeDocker) VolumeInspect(ctx context.Context, name string) (dockervolume.Volume, error) {
	if err, ok := f.inspectErr[name]; ok {
		return dockervolume.Volume{}, err
	}
	if v, ok := f.inspect[name]; ok {
		return v, nil
	}
	return dockervolume.Volume{}, errdefs.ErrNotFound
}

func (f *fakeDocker) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	volumeFilter := opts.Filters.Get("volume")

	out := make([]container.Summary, 0, len(f.containers))
	for _, c := range f.containers {
		if !matchesFilters(c.Labels, opts.Filters) {
			continue
		}
		if len(volumeFilter) > 0 && !containerHasVolume(f.mounts[c.ID], volumeFilter) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func containerHasVolume(mounts []container.MountPoint, wanted []string) bool {
	for _, m := range mounts {
		if m.Type != "volume" {
			continue
		}
		for _, w := range wanted {
			if m.Name == w {
				return true
			}
		}
	}
	return false
}

func (f *fakeDocker) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	name, ok := f.names[id]
	if !ok {
		return container.InspectResponse{}, errdefs.ErrNotFound
	}
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{ID: id, Name: name},
		Mounts:            f.mounts[id],
	}, nil
}

func (f *fakeDocker) VolumeCreate(ctx context.Context, opts dockervolume.CreateOptions) (dockervolume.Volume, error) {
	panic("unexpected VolumeCreate call")
}

func (f *fakeDocker) VolumeRemove(ctx context.Context, id string, force bool) error {
	panic("unexpected VolumeRemove call")
}

func (f *fakeDocker) VolumesPrune(ctx context.Context, args filters.Args) (dockervolume.PruneReport, error) {
	panic("unexpected VolumesPrune call")
}

func (f *fakeDocker) ImageBuild(ctx context.Context, buildContext io.Reader, opts dockerbuild.ImageBuildOptions) (dockerbuild.ImageBuildResponse, error) {
	panic("unexpected")
}

func (f *fakeDocker) ImageList(ctx context.Context, opts image.ListOptions) ([]image.Summary, error) {
	panic("unexpected")
}

func (f *fakeDocker) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	panic("unexpected")
}

func (f *fakeDocker) ImageRemove(ctx context.Context, id string, opts image.RemoveOptions) ([]image.DeleteResponse, error) {
	panic("unexpected")
}

func (f *fakeDocker) ContainerCreate(ctx context.Context, c *container.Config, h *container.HostConfig, n *network.NetworkingConfig, p *ocispec.Platform, name string) (container.CreateResponse, error) {
	panic("unexpected")
}

func (f *fakeDocker) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	panic("unexpected")
}

func (f *fakeDocker) ContainerStop(ctx context.Context, id string, opts container.StopOptions) error {
	panic("unexpected")
}

func (f *fakeDocker) ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	panic("unexpected")
}

func (f *fakeDocker) ContainerStatsOneShot(ctx context.Context, id string) (container.StatsResponseReader, error) {
	panic("unexpected")
}

func (f *fakeDocker) ContainerAttach(ctx context.Context, id string, opts container.AttachOptions) (types.HijackedResponse, error) {
	panic("unexpected")
}

func (f *fakeDocker) ContainerResize(ctx context.Context, id string, opts container.ResizeOptions) error {
	panic("unexpected")
}

func (f *fakeDocker) NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	panic("unexpected")
}

func (f *fakeDocker) NetworkRemove(ctx context.Context, id string) error { panic("unexpected") }
func (f *fakeDocker) NetworkList(ctx context.Context, opts network.ListOptions) ([]network.Summary, error) {
	panic("unexpected")
}
func (f *fakeDocker) ServerVersion(ctx context.Context) (types.Version, error) { panic("unexpected") }
func (f *fakeDocker) Close() error                                             { return nil }

func matchesFilters(labels map[string]string, args filters.Args) bool {
	if args.Len() == 0 {
		return true
	}
	for _, key := range args.Keys() {
		if key != "label" && key != "volume" {
			panic("fakeDocker: unsupported filter key: " + key)
		}
	}
	for _, want := range args.Get("label") {
		k, v, ok := splitLabel(want)
		if !ok {
			continue
		}
		if labels[k] != v {
			return false
		}
	}
	return true
}

func splitLabel(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

func managed(project, owner, ownerKind, logical string) map[string]string {
	return map[string]string{
		deploy.LabelManaged:   "true",
		deploy.LabelProject:   project,
		deploy.LabelOwner:     owner,
		deploy.LabelOwnerKind: ownerKind,
		deploy.LabelVolume:    logical,
		deploy.LabelCreatedAt: "2026-04-14T10:00:00Z",
		deploy.LabelSchema:    deploy.SchemaVersion,
	}
}

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("filters_out_unmanaged_and_other_projects", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{
			volumes: []*dockervolume.Volume{
				{Name: "foo_lobby_world", Driver: "local", Labels: managed("foo", "foo-lobby", "server", "world")},
				{Name: "bar_db_data", Driver: "local", Labels: managed("bar", "bar-db", "service", "data")},
				{Name: "legacy", Driver: "local", Labels: map[string]string{}}, // unmanaged
			},
		}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		got, err := svc.List(context.Background(), "foo")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "foo_lobby_world", got[0].Name)
		assert.Equal(t, "foo", got[0].Project)
		assert.Equal(t, "world", got[0].Logical)
		assert.Equal(t, SizeUnknown, got[0].SizeBytes)
	})

	t.Run("merges_in_use_by_from_containers", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{
			volumes: []*dockervolume.Volume{
				{Name: "p_a_world", Labels: managed("p", "p-a", "server", "world")},
				{Name: "p_b_data", Labels: managed("p", "p-b", "service", "data")},
			},
			containers: []container.Summary{
				{ID: "c1", Labels: managed("p", "p-a", "server", "")},
			},
			names: map[string]string{"c1": "/p-a"},
			mounts: map[string][]container.MountPoint{
				"c1": {{Type: "volume", Name: "p_a_world"}},
			},
		}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		got, err := svc.List(context.Background(), "p")
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"p-a"}, got[0].InUseBy)
		assert.Empty(t, got[1].InUseBy)
	})

	t.Run("continues_when_container_scan_fails", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{
			volumes: []*dockervolume.Volume{
				{Name: "p_a_world", Labels: managed("p", "p-a", "server", "world")},
			},
			containers: []container.Summary{{ID: "ghost", Labels: managed("p", "p-a", "server", "")}},
		}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		got, err := svc.List(context.Background(), "p")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Empty(t, got[0].InUseBy)
	})

	t.Run("list_error_surfaces", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("docker down")
		fd := &fakeDocker{listErr: boom}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		_, err := svc.List(context.Background(), "")
		require.ErrorIs(t, err, boom)
	})
}

func TestInspect(t *testing.T) {
	t.Parallel()

	t.Run("returns_managed_volume", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{
			inspect: map[string]dockervolume.Volume{
				"p_a_world": {
					Name:       "p_a_world",
					Driver:     "local",
					Mountpoint: "/var/lib/docker/volumes/p_a_world/_data",
					Labels:     managed("p", "p-a", "server", "world"),
					CreatedAt:  "2026-04-14T10:00:00Z",
				},
			},
		}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		got, err := svc.Inspect(context.Background(), "p_a_world")
		require.NoError(t, err)
		assert.Equal(t, "world", got.Logical)
		assert.Equal(t, time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC), got.CreatedAt)
	})

	t.Run("unmanaged_volume_returns_not_found", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{
			inspect: map[string]dockervolume.Volume{
				"raw": {Name: "raw", Labels: map[string]string{}},
			},
		}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		_, err := svc.Inspect(context.Background(), "raw")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("missing_volume_returns_not_found", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		_, err := svc.Inspect(context.Background(), "nope")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("populates_in_use_by_via_volume_filter", func(t *testing.T) {
		t.Parallel()
		fd := &fakeDocker{
			inspect: map[string]dockervolume.Volume{
				"p_a_world": {Name: "p_a_world", Labels: managed("p", "p-a", "server", "world")},
			},
			containers: []container.Summary{
				{ID: "c1", Names: []string{"/p-a"}, Labels: managed("p", "p-a", "server", "")},
				{ID: "c2", Names: []string{"/p-b"}, Labels: managed("p", "p-b", "server", "")},
			},
			mounts: map[string][]container.MountPoint{
				"c1": {{Type: "volume", Name: "p_a_world"}},
				"c2": {{Type: "volume", Name: "p_b_data"}},
			},
		}
		svc := New(fd, slog.New(slog.NewTextHandler(io.Discard, nil)))

		got, err := svc.Inspect(context.Background(), "p_a_world")
		require.NoError(t, err)
		assert.Equal(t, []string{"p-a"}, got.InUseBy)
	})
}
