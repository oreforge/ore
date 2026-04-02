package console

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"github.com/oreforge/ore/internal/docker"
)

type DockerConn struct {
	hijacked  types.HijackedResponse
	client    docker.Client
	container string
	buf       []byte
}

func NewDockerConn(hijacked types.HijackedResponse, client docker.Client, containerName string) *DockerConn {
	return &DockerConn{
		hijacked:  hijacked,
		client:    client,
		container: containerName,
		buf:       make([]byte, 4096),
	}
}

func (d *DockerConn) Read(_ context.Context) ([]byte, error) {
	n, err := d.hijacked.Conn.Read(d.buf)
	if n > 0 {
		return d.buf[:n], nil
	}
	return nil, err
}

func (d *DockerConn) Write(_ context.Context, data []byte) error {
	_, err := d.hijacked.Conn.Write(data)
	return err
}

func (d *DockerConn) Resize(ctx context.Context, width, height int) error {
	return d.client.ContainerResize(ctx, d.container, container.ResizeOptions{
		Width:  uint(width),
		Height: uint(height),
	})
}

func (d *DockerConn) Close() error {
	return d.hijacked.CloseWrite()
}
