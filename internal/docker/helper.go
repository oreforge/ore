package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stdcopy"
)

const HelperImage = "alpine:3"

var (
	helperImageMu    sync.Mutex
	helperImageCache = map[string]struct{}{}
)

type HelperSpec struct {
	Image   string
	Cmd     []string
	Binds   []string
	WorkDir string
	Name    string
	Env     []string
}

type HelperResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int64
}

func ensureImage(ctx context.Context, d Client, ref string, logger *slog.Logger) error {
	helperImageMu.Lock()
	if _, ok := helperImageCache[ref]; ok {
		helperImageMu.Unlock()
		return nil
	}
	helperImageMu.Unlock()

	images, err := d.ImageList(ctx, image.ListOptions{Filters: filters.NewArgs(filters.Arg("reference", ref))})
	if err == nil && len(images) > 0 {
		helperImageMu.Lock()
		helperImageCache[ref] = struct{}{}
		helperImageMu.Unlock()
		return nil
	}

	logger.Debug("pulling helper image", "ref", ref)
	rc, err := d.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("draining image pull stream: %w", err)
	}

	helperImageMu.Lock()
	helperImageCache[ref] = struct{}{}
	helperImageMu.Unlock()
	return nil
}

func RunHelper(ctx context.Context, d Client, logger *slog.Logger, spec HelperSpec) (*HelperResult, error) {
	img := spec.Image
	if img == "" {
		img = HelperImage
	}

	if err := ensureImage(ctx, d, img, logger); err != nil {
		return nil, err
	}

	cfg := &container.Config{
		Image:      img,
		Cmd:        spec.Cmd,
		Env:        spec.Env,
		WorkingDir: spec.WorkDir,
		Tty:        false,
		Labels: map[string]string{
			"ore.helper": "true",
		},
	}

	hostCfg := &container.HostConfig{
		AutoRemove: false,
		Binds:      spec.Binds,
	}

	created, err := d.ContainerCreate(ctx, cfg, hostCfg, nil, nil, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("creating helper container: %w", err)
	}
	id := created.ID
	defer func() {
		if rmErr := d.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true}); rmErr != nil && !cerrdefs.IsNotFound(rmErr) {
			logger.Warn("failed to remove helper container", "id", id, "error", rmErr)
		}
	}()

	if err := d.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("starting helper container: %w", err)
	}

	statusCh, errCh := d.ContainerWait(ctx, id, container.WaitConditionNotRunning)

	var exitCode int64
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("waiting for helper container: %w", err)
		}
	case st := <-statusCh:
		exitCode = st.StatusCode
		if st.Error != nil && st.Error.Message != "" {
			return &HelperResult{ExitCode: exitCode}, errors.New(st.Error.Message)
		}
	}

	logs, err := d.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return &HelperResult{ExitCode: exitCode}, fmt.Errorf("reading helper logs: %w", err)
	}
	defer func() { _ = logs.Close() }()

	var stdout, stderr strings.Builder
	if _, err := stdcopy.StdCopy(&stringBuilderWriter{&stdout}, &stringBuilderWriter{&stderr}, logs); err != nil {
		return &HelperResult{ExitCode: exitCode}, fmt.Errorf("decoding helper output: %w", err)
	}

	res := &HelperResult{
		Stdout:   []byte(stdout.String()),
		Stderr:   []byte(stderr.String()),
		ExitCode: exitCode,
	}
	if exitCode != 0 {
		return res, fmt.Errorf("helper exited with code %d: %s", exitCode, strings.TrimSpace(stderr.String()))
	}
	return res, nil
}

type stringBuilderWriter struct{ b *strings.Builder }

func (w *stringBuilderWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

type HelperStream struct {
	Stdout io.ReadCloser
	Wait   func() (int64, string, error)
}

func RunHelperWithStdin(ctx context.Context, d Client, logger *slog.Logger, spec HelperSpec, stdin io.Reader) error {
	img := spec.Image
	if img == "" {
		img = HelperImage
	}
	if err := ensureImage(ctx, d, img, logger); err != nil {
		return err
	}

	cfg := &container.Config{
		Image:      img,
		Cmd:        spec.Cmd,
		Env:        spec.Env,
		WorkingDir: spec.WorkDir,
		OpenStdin:  true,
		StdinOnce:  true,
		Tty:        false,
		Labels:     map[string]string{"ore.helper": "true"},
	}
	hostCfg := &container.HostConfig{Binds: spec.Binds}

	created, err := d.ContainerCreate(ctx, cfg, hostCfg, nil, nil, spec.Name)
	if err != nil {
		return fmt.Errorf("creating helper container: %w", err)
	}
	id := created.ID
	defer func() {
		if rmErr := d.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true}); rmErr != nil && !cerrdefs.IsNotFound(rmErr) {
			logger.Warn("failed to remove helper container", "id", id, "error", rmErr)
		}
	}()

	attach, err := d.ContainerAttach(ctx, id, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return fmt.Errorf("attaching helper: %w", err)
	}
	defer attach.Close()

	if err := d.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting helper: %w", err)
	}

	statusCh, errCh := d.ContainerWait(ctx, id, container.WaitConditionNotRunning)

	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(attach.Conn, stdin)
		_ = attach.CloseWrite()
		copyDone <- copyErr
	}()

	stderrBuf := &strings.Builder{}
	go func() {
		_, _ = stdcopy.StdCopy(io.Discard, &stringBuilderWriter{stderrBuf}, attach.Reader)
	}()

	if err := <-copyDone; err != nil {
		return fmt.Errorf("writing to helper stdin: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("waiting for helper: %w", err)
		}
	case st := <-statusCh:
		if st.Error != nil && st.Error.Message != "" {
			return errors.New(st.Error.Message)
		}
		if st.StatusCode != 0 {
			return fmt.Errorf("helper exited with code %d: %s", st.StatusCode, strings.TrimSpace(stderrBuf.String()))
		}
	}
	return nil
}

func StartHelperStream(ctx context.Context, d Client, logger *slog.Logger, spec HelperSpec) (*HelperStream, error) {
	img := spec.Image
	if img == "" {
		img = HelperImage
	}
	if err := ensureImage(ctx, d, img, logger); err != nil {
		return nil, err
	}

	cfg := &container.Config{
		Image:      img,
		Cmd:        spec.Cmd,
		Env:        spec.Env,
		WorkingDir: spec.WorkDir,
		Tty:        false,
		Labels:     map[string]string{"ore.helper": "true"},
	}
	hostCfg := &container.HostConfig{Binds: spec.Binds}

	created, err := d.ContainerCreate(ctx, cfg, hostCfg, nil, nil, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("creating helper container: %w", err)
	}
	id := created.ID

	removeContainer := func() {
		if rmErr := d.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true}); rmErr != nil && !cerrdefs.IsNotFound(rmErr) {
			logger.Warn("failed to remove helper container", "id", id, "error", rmErr)
		}
	}

	if err := d.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		removeContainer()
		return nil, fmt.Errorf("starting helper container: %w", err)
	}

	logs, err := d.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		removeContainer()
		return nil, fmt.Errorf("attaching helper logs: %w", err)
	}

	stdoutR, stdoutW := io.Pipe()
	stderrBuf := &strings.Builder{}

	go func() {
		_, copyErr := stdcopy.StdCopy(stdoutW, &stringBuilderWriter{stderrBuf}, logs)
		_ = logs.Close()
		_ = stdoutW.CloseWithError(copyErr)
	}()

	statusCh, errCh := d.ContainerWait(ctx, id, container.WaitConditionNotRunning)

	wait := func() (int64, string, error) {
		defer removeContainer()
		select {
		case <-ctx.Done():
			return 0, stderrBuf.String(), ctx.Err()
		case err := <-errCh:
			if err != nil {
				return 0, stderrBuf.String(), fmt.Errorf("waiting for helper container: %w", err)
			}
			return 0, stderrBuf.String(), nil
		case st := <-statusCh:
			if st.Error != nil && st.Error.Message != "" {
				return st.StatusCode, stderrBuf.String(), errors.New(st.Error.Message)
			}
			if st.StatusCode != 0 {
				return st.StatusCode, stderrBuf.String(), fmt.Errorf("helper exited with code %d: %s", st.StatusCode, strings.TrimSpace(stderrBuf.String()))
			}
			return 0, stderrBuf.String(), nil
		}
	}

	return &HelperStream{Stdout: stdoutR, Wait: wait}, nil
}
