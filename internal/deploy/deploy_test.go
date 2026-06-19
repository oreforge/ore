package deploy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
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

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/spec"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeClient struct {
	mu sync.Mutex

	images       []image.Summary
	inspect      map[string]container.InspectResponse
	imageListErr error
	pullErr      error
	createErr    map[string]error

	startOrder    []string
	created       []string
	removed       []string
	networkList   []network.Summary
	networkCreate []string
	networkRemove []string
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		inspect:   make(map[string]container.InspectResponse),
		createErr: make(map[string]error),
	}
}

func runningInspect(img string) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			State: &container.State{Status: "running", Running: true},
		},
		Config: &container.Config{Image: img},
	}
}

func (f *fakeClient) ImageList(ctx context.Context, opts image.ListOptions) ([]image.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.imageListErr != nil {
		return nil, f.imageListErr
	}
	return f.images, nil
}

func (f *fakeClient) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pullErr != nil {
		return nil, f.pullErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeClient) ContainerCreate(ctx context.Context, c *container.Config, h *container.HostConfig, n *network.NetworkingConfig, p *ocispec.Platform, name string) (container.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.createErr[name]; err != nil {
		return container.CreateResponse{}, err
	}
	f.created = append(f.created, name)
	if _, ok := f.inspect[name]; !ok {
		f.inspect[name] = runningInspect(c.Image)
	}
	return container.CreateResponse{ID: name}, nil
}

func (f *fakeClient) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startOrder = append(f.startOrder, id)
	return nil
}

func (f *fakeClient) ContainerStop(ctx context.Context, id string, opts container.StopOptions) error {
	return nil
}

func (f *fakeClient) ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeClient) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if info, ok := f.inspect[id]; ok {
		return info, nil
	}
	return container.InspectResponse{}, errdefs.ErrNotFound
}

func (f *fakeClient) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	return nil, nil
}

func (f *fakeClient) NetworkList(ctx context.Context, opts network.ListOptions) ([]network.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.networkList, nil
}

func (f *fakeClient) NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.networkCreate = append(f.networkCreate, name)
	return network.CreateResponse{ID: name}, nil
}

func (f *fakeClient) NetworkRemove(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.networkRemove = append(f.networkRemove, id)
	return nil
}

func (f *fakeClient) VolumeCreate(ctx context.Context, opts dockervolume.CreateOptions) (dockervolume.Volume, error) {
	return dockervolume.Volume{Name: opts.Name}, nil
}

func (f *fakeClient) ImageBuild(ctx context.Context, buildContext io.Reader, opts dockerbuild.ImageBuildOptions) (dockerbuild.ImageBuildResponse, error) {
	panic("unexpected ImageBuild call")
}

func (f *fakeClient) ImageRemove(ctx context.Context, id string, opts image.RemoveOptions) ([]image.DeleteResponse, error) {
	panic("unexpected ImageRemove call")
}

func (f *fakeClient) ContainerPause(ctx context.Context, id string) error {
	panic("unexpected ContainerPause call")
}

func (f *fakeClient) ContainerUnpause(ctx context.Context, id string) error {
	panic("unexpected ContainerUnpause call")
}

func (f *fakeClient) ContainerStatsOneShot(ctx context.Context, id string) (container.StatsResponseReader, error) {
	panic("unexpected ContainerStatsOneShot call")
}

func (f *fakeClient) ContainerAttach(ctx context.Context, id string, opts container.AttachOptions) (types.HijackedResponse, error) {
	panic("unexpected ContainerAttach call")
}

func (f *fakeClient) ContainerResize(ctx context.Context, id string, opts container.ResizeOptions) error {
	panic("unexpected ContainerResize call")
}

func (f *fakeClient) ContainerWait(ctx context.Context, id string, cond container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	panic("unexpected ContainerWait call")
}

func (f *fakeClient) ContainerLogs(ctx context.Context, id string, opts container.LogsOptions) (io.ReadCloser, error) {
	panic("unexpected ContainerLogs call")
}

func (f *fakeClient) VolumeRemove(ctx context.Context, id string, force bool) error {
	panic("unexpected VolumeRemove call")
}

func (f *fakeClient) VolumeList(ctx context.Context, opts dockervolume.ListOptions) (dockervolume.ListResponse, error) {
	panic("unexpected VolumeList call")
}

func (f *fakeClient) VolumeInspect(ctx context.Context, id string) (dockervolume.Volume, error) {
	panic("unexpected VolumeInspect call")
}

func (f *fakeClient) VolumesPrune(ctx context.Context, args filters.Args) (dockervolume.PruneReport, error) {
	panic("unexpected VolumesPrune call")
}

func (f *fakeClient) DiskUsage(ctx context.Context, opts types.DiskUsageOptions) (types.DiskUsage, error) {
	panic("unexpected DiskUsage call")
}

func (f *fakeClient) ServerVersion(ctx context.Context) (types.Version, error) {
	panic("unexpected ServerVersion call")
}

func (f *fakeClient) Close() error { return nil }

func (f *fakeClient) startedSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.startOrder))
	copy(out, f.startOrder)
	return out
}

func TestUpSingleServer(t *testing.T) {
	t.Parallel()

	fc := newFakeClient()
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network: "net",
		Servers: []spec.Server{{Name: "lobby", Software: "paper:1.20.6"}},
	}
	images := map[string]build.Result{
		"lobby": {ImageTag: "ore/lobby:abc", HealthTimeout: time.Second},
	}

	state, err := d.Up(context.Background(), cfg, images, UpOptions{Force: true})
	require.NoError(t, err)
	require.NotNil(t, state)

	srvState, ok := state.Servers["lobby"]
	require.True(t, ok)
	assert.Equal(t, "ore/lobby:abc", srvState.ImageTag)
	assert.Equal(t, spec.ServerHash(&cfg.Servers[0], "ore/lobby:abc"), srvState.ConfigHash)
	assert.Empty(t, state.Services)

	assert.Equal(t, []string{"lobby"}, fc.startedSnapshot())
	assert.Contains(t, fc.created, "lobby")
	assert.Contains(t, fc.networkCreate, "net")
}

func TestUpDependencyOrdering(t *testing.T) {
	t.Parallel()

	fc := newFakeClient()
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network: "net",
		Servers: []spec.Server{
			{
				Name:      "lobby",
				Software:  "paper:1.20.6",
				DependsOn: []spec.Dependency{{Name: "db", Condition: spec.ConditionStarted}},
			},
		},
		Services: []spec.Service{
			{Name: "db", Image: "postgres:16"},
		},
	}
	images := map[string]build.Result{
		"lobby": {ImageTag: "ore/lobby:abc"},
	}

	groups := spec.TopologicalOrder(cfg)
	require.Len(t, groups, 2)
	assert.Equal(t, []string{"db"}, groups[0].Services)
	assert.Equal(t, []string{"lobby"}, groups[1].Servers)

	state, err := d.Up(context.Background(), cfg, images, UpOptions{Force: true})
	require.NoError(t, err)

	order := fc.startedSnapshot()
	require.Equal(t, []string{"db", "lobby"}, order)

	require.Contains(t, state.Servers, "lobby")
	require.Contains(t, state.Services, "db")
	assert.Equal(t, "postgres:16", state.Services["db"].Image)
}

func TestUpMissingImageTag(t *testing.T) {
	t.Parallel()

	fc := newFakeClient()
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network: "net",
		Servers: []spec.Server{{Name: "lobby"}},
	}

	t.Run("server_not_in_images", func(t *testing.T) {
		t.Parallel()
		_, err := d.Up(context.Background(), cfg, map[string]build.Result{}, UpOptions{Force: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no image found for server lobby")
	})

	t.Run("empty_tag", func(t *testing.T) {
		t.Parallel()
		images := map[string]build.Result{"lobby": {ImageTag: ""}}
		_, err := d.Up(context.Background(), cfg, images, UpOptions{Force: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no image tag for server lobby")
	})
}

func TestUpImageListError(t *testing.T) {
	t.Parallel()

	boom := errors.New("registry unreachable")
	fc := newFakeClient()
	fc.imageListErr = boom
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network:  "net",
		Services: []spec.Service{{Name: "db", Image: "postgres:16"}},
	}

	_, err := d.Up(context.Background(), cfg, nil, UpOptions{Force: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing images")
}

func TestUpPullError(t *testing.T) {
	t.Parallel()

	boom := errors.New("pull denied")
	fc := newFakeClient()
	fc.pullErr = boom
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network:  "net",
		Services: []spec.Service{{Name: "db", Image: "postgres:16"}},
	}

	_, err := d.Up(context.Background(), cfg, nil, UpOptions{Force: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pull denied")
}

func TestUpStartContainerError(t *testing.T) {
	t.Parallel()

	boom := errors.New("create rejected")
	fc := newFakeClient()
	fc.createErr["lobby"] = boom
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network: "net",
		Servers: []spec.Server{{Name: "lobby"}},
	}
	images := map[string]build.Result{"lobby": {ImageTag: "ore/lobby:abc"}}

	_, err := d.Up(context.Background(), cfg, images, UpOptions{Force: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starting lobby")
}

func TestUnchanged(t *testing.T) {
	t.Parallel()

	const (
		name = "lobby"
		img  = "ore/lobby:abc"
		hash = "hash1"
	)

	prevState := &State{
		Servers: map[string]ServerState{name: {ImageTag: img, ConfigHash: hash}},
	}

	t.Run("force_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = runningInspect(img)
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, hash, UpOptions{PrevState: prevState, Force: true})
		assert.False(t, got)
	})

	t.Run("nil_prev_state_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = runningInspect(img)
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, hash, UpOptions{})
		assert.False(t, got)
	})

	t.Run("image_differs_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = runningInspect(img)
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, "ore/lobby:old", hash, hash, UpOptions{PrevState: prevState})
		assert.False(t, got)
	})

	t.Run("hash_differs_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = runningInspect(img)
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, "other", UpOptions{PrevState: prevState})
		assert.False(t, got)
	})

	t.Run("inspect_error_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, hash, UpOptions{PrevState: prevState})
		assert.False(t, got)
	})

	t.Run("not_running_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{State: &container.State{Status: "exited"}},
			Config:            &container.Config{Image: img},
		}
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, hash, UpOptions{PrevState: prevState})
		assert.False(t, got)
	})

	t.Run("running_image_mismatch_returns_false", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = runningInspect("ore/lobby:stale")
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, hash, UpOptions{PrevState: prevState})
		assert.False(t, got)
	})

	t.Run("matching_and_running_returns_true", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClient()
		fc.inspect[name] = runningInspect(img)
		d := New(fc, discardLogger(), nil, false)
		got := d.unchanged(context.Background(), name, img, img, hash, hash, UpOptions{PrevState: prevState})
		assert.True(t, got)
	})
}

func TestUpSkipsUnchangedServer(t *testing.T) {
	t.Parallel()

	fc := newFakeClient()
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network: "net",
		Servers: []spec.Server{{Name: "lobby", Software: "paper:1.20.6"}},
	}
	tag := "ore/lobby:abc"
	images := map[string]build.Result{"lobby": {ImageTag: tag}}
	hash := spec.ServerHash(&cfg.Servers[0], tag)

	fc.inspect["lobby"] = runningInspect(tag)

	prev := &State{
		Servers:  map[string]ServerState{"lobby": {ImageTag: tag, ConfigHash: hash}},
		Services: map[string]ServiceState{},
	}

	state, err := d.Up(context.Background(), cfg, images, UpOptions{PrevState: prev})
	require.NoError(t, err)

	assert.Empty(t, fc.startedSnapshot())
	assert.Empty(t, fc.created)
	require.Contains(t, state.Servers, "lobby")
	assert.Equal(t, tag, state.Servers["lobby"].ImageTag)
}

func TestDown(t *testing.T) {
	t.Parallel()

	fc := newFakeClient()
	d := New(fc, discardLogger(), nil, false)

	cfg := &spec.Network{
		Network: "net",
		Servers: []spec.Server{{Name: "lobby"}},
		Services: []spec.Service{
			{Name: "db", Image: "postgres:16"},
		},
	}

	require.NoError(t, d.Down(context.Background(), cfg))

	assert.Contains(t, fc.removed, "lobby")
	assert.Contains(t, fc.removed, "db")
	assert.Contains(t, fc.networkRemove, "net")
}
