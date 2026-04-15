package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/oreforge/ore/internal/docker"
)

type SnapshotStream struct {
	Reader io.ReadCloser
	Wait   func() error
}

type Snapshotter interface {
	Snapshot(ctx context.Context, volumeName string, logger *slog.Logger) (SnapshotStream, error)
	Extract(ctx context.Context, volumeName string, src io.Reader, logger *slog.Logger) error
}

type HelperSnapshotter struct {
	docker docker.Client
}

func NewHelperSnapshotter(d docker.Client) *HelperSnapshotter { return &HelperSnapshotter{docker: d} }

func (h *HelperSnapshotter) Snapshot(ctx context.Context, volumeName string, logger *slog.Logger) (SnapshotStream, error) {
	hs, err := docker.StartHelperStream(ctx, h.docker, logger, docker.HelperSpec{
		Cmd:     []string{"sh", "-c", "tar -C /volume -cf - ."},
		Binds:   []string{volumeName + ":/volume:ro"},
		WorkDir: "/volume",
	})
	if err != nil {
		return SnapshotStream{}, fmt.Errorf("starting snapshot helper: %w", err)
	}
	wait := func() error {
		_, _, werr := hs.Wait()
		return werr
	}
	return SnapshotStream{Reader: hs.Stdout, Wait: wait}, nil
}

func (h *HelperSnapshotter) Extract(ctx context.Context, volumeName string, src io.Reader, logger *slog.Logger) error {
	return docker.RunHelperWithStdin(ctx, h.docker, logger, docker.HelperSpec{
		Cmd:   []string{"sh", "-c", "find /volume -mindepth 1 -delete && tar -xf - -C /volume"},
		Binds: []string{volumeName + ":/volume"},
	}, src)
}
