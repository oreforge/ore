package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	dockerbuild "github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type ImageClient interface {
	ImageBuild(ctx context.Context, buildContext io.Reader, options dockerbuild.ImageBuildOptions) (dockerbuild.ImageBuildResponse, error)
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
}

type ContainerClient interface {
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerPause(ctx context.Context, containerID string) error
	ContainerUnpause(ctx context.Context, containerID string) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerStatsOneShot(ctx context.Context, containerID string) (container.StatsResponseReader, error)
	ContainerAttach(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error)
	ContainerResize(ctx context.Context, container string, options container.ResizeOptions) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
}

type NetworkClient interface {
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(ctx context.Context, networkID string) error
	NetworkList(ctx context.Context, options network.ListOptions) ([]network.Summary, error)
}

type VolumeClient interface {
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumesPrune(ctx context.Context, pruneFilter filters.Args) (volume.PruneReport, error)
}

type SystemClient interface {
	DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error)
}

type Client interface {
	ImageClient
	ContainerClient
	NetworkClient
	VolumeClient
	SystemClient
	ServerVersion(ctx context.Context) (types.Version, error)
	Close() error
}

func New(ctx context.Context) (*dockerclient.Client, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}

	if _, err := cli.Ping(ctx); err != nil {
		_ = cli.Close()
		return nil, err
	}

	return cli, nil
}
