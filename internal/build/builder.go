package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	bkclient "github.com/moby/buildkit/client"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/oreforge/ore/internal/cache"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/resolver"
	"github.com/oreforge/ore/internal/resolver/providers"
	"github.com/oreforge/ore/internal/spec"
)

type Options struct {
	NoCache    bool
	ForceBuild bool
}

type Result struct {
	ImageTag      string
	HealthTimeout time.Duration
	Cached        bool
}

type Builder struct {
	docker    docker.Client
	bk        *bkclient.Client
	registry  *providers.Registry
	logger    *slog.Logger
	workDir   *cache.Manager
	opts      Options
	fetchOnce singleflight.Group
}

func NewBuilder(dockerClient docker.Client, bk *bkclient.Client, registry *providers.Registry, logger *slog.Logger, workDir *cache.Manager, opts Options) *Builder {
	return &Builder{
		docker:   dockerClient,
		bk:       bk,
		registry: registry,
		logger:   logger,
		workDir:  workDir,
		opts:     opts,
	}
}

func (b *Builder) BuildAll(ctx context.Context, cfg *spec.NetworkSpec, repoRoot string) (map[string]Result, error) {
	var mu sync.Mutex
	images := make(map[string]Result, len(cfg.Servers))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	for _, srv := range cfg.Servers {
		g.Go(func() error {
			res, err := b.Build(ctx, &srv, repoRoot)
			if err != nil {
				return fmt.Errorf("building %s: %w", srv.Name, err)
			}
			mu.Lock()
			images[srv.Name] = res
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if b.workDir != nil {
		if err := b.workDir.SaveManifest(); err != nil {
			b.logger.Warn("failed to save manifest", "error", err)
		}
	}

	return images, nil
}

func (b *Builder) Build(ctx context.Context, srv *spec.ServerSpec, repoRoot string) (Result, error) {
	startedAt := time.Now()

	b.logger.Info("resolving software", "server", srv.Name, "software", srv.Software)

	platform, err := b.platform(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("detecting docker platform: %w", err)
	}

	artifact, err := b.registry.Resolve(ctx, srv.Software, platform)
	if err != nil {
		return Result{}, fmt.Errorf("resolving software: %w", err)
	}

	serverDir := filepath.Join(repoRoot, srv.Dir)

	cacheKey, err := CacheKey(srv.Software, artifact.BuildID, serverDir)
	if err != nil {
		return Result{}, fmt.Errorf("computing cache key: %w", err)
	}

	imageTag := fmt.Sprintf("ore/%s:%s", srv.Name, cacheKey)
	result := Result{ImageTag: imageTag, HealthTimeout: artifact.HealthTimeout}

	if !b.opts.ForceBuild && b.imageExists(ctx, imageTag) {
		b.logger.Info("image cached, skipping build", "server", srv.Name, "tag", imageTag)
		result.Cached = true
		return result, nil
	}
	if b.opts.ForceBuild {
		b.logger.Info("forcing image build", "server", srv.Name)
	}

	binaryData, cached, err := b.fetchBinary(ctx, srv, artifact)
	if err != nil {
		return Result{}, err
	}

	if artifact.Runtime == nil {
		return Result{}, fmt.Errorf("artifact for %s has no runtime configured", srv.Software)
	}

	dfOpts := DockerfileOptions{
		Runtime:       artifact.Runtime,
		ExtraArgs:     artifact.ExtraArgs,
		HealthRetries: artifact.HealthRetries,
	}
	dockerfile := GenerateDockerfile(dfOpts)
	entrypoint := artifact.Runtime.Entrypoint()
	binaryName := artifact.Runtime.BinaryName()
	binaryMode := os.FileMode(artifact.Runtime.BinaryMode())

	buildDir, cleanup, err := b.prepareBuildDir(srv.Name, cacheKey, serverDir, dockerfile, entrypoint, binaryName, binaryData, binaryMode)
	if err != nil {
		return Result{}, fmt.Errorf("preparing build context: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	b.logger.Info("building image", "server", srv.Name, "tag", imageTag)

	if err := b.buildWithBuildKit(ctx, buildDir, imageTag, srv.Name, cacheKey); err != nil {
		return Result{}, err
	}

	duration := time.Since(startedAt)
	b.logger.Info("image built", "server", srv.Name, "tag", imageTag, "duration", duration)

	if b.workDir != nil {
		meta := cache.BuildMetadata{
			ServerName:   srv.Name,
			SoftwareID:   srv.Software,
			ArtifactURL:  artifact.URL,
			ImageTag:     imageTag,
			CacheKey:     cacheKey,
			Runtime:      artifact.Runtime.Name(),
			BinaryCached: cached,
			StartedAt:    startedAt,
			DurationMs:   duration.Milliseconds(),
		}
		if err := b.workDir.WriteMetadata(srv.Name, cacheKey, meta); err != nil {
			b.logger.Warn("failed to write build metadata", "error", err)
		}
	}

	return result, nil
}

func (b *Builder) prepareBuildDir(serverName, cacheKey, serverDir, dockerfile, entrypoint, binaryName string, binaryData []byte, binaryMode os.FileMode) (string, func(), error) {
	if b.workDir != nil {
		b.workDir.CleanOldBuilds(serverName, cacheKey)
		if err := b.workDir.WriteDockerfile(serverName, cacheKey, dockerfile); err != nil {
			return "", nil, fmt.Errorf("writing Dockerfile: %w", err)
		}
		if err := b.workDir.WriteDataDir(serverName, cacheKey, serverDir); err != nil {
			return "", nil, fmt.Errorf("writing data dir: %w", err)
		}
		if err := b.workDir.WriteBinary(serverName, cacheKey, binaryName, binaryData, binaryMode); err != nil {
			return "", nil, fmt.Errorf("writing binary: %w", err)
		}
		if entrypoint != "" {
			if err := b.workDir.WriteEntrypoint(serverName, cacheKey, []byte(entrypoint)); err != nil {
				return "", nil, fmt.Errorf("writing entrypoint: %w", err)
			}
		}
		dir, err := b.workDir.BuildDir(serverName, cacheKey)
		return dir, nil, err
	}

	tmpDir, err := os.MkdirTemp("", "ore-build-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, binaryName), binaryData, binaryMode); err != nil {
		cleanup()
		return "", nil, err
	}
	if entrypoint != "" {
		if err := os.WriteFile(filepath.Join(tmpDir, "entrypoint.sh"), []byte(entrypoint), 0o755); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	if err := cache.CopyDir(serverDir, filepath.Join(tmpDir, "data")); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmpDir, cleanup, nil
}

func (b *Builder) buildWithBuildKit(ctx context.Context, buildDir, imageTag, serverName, cacheKey string) error {
	ctxMount, err := fsutil.NewFS(buildDir)
	if err != nil {
		return fmt.Errorf("creating build context mount: %w", err)
	}

	pipeR, pipeW := io.Pipe()
	ch := make(chan *bkclient.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		defer func() { _ = pipeW.Close() }()
		_, err := b.bk.Solve(ctx, nil, bkclient.SolveOpt{
			Frontend: "dockerfile.v0",
			FrontendAttrs: map[string]string{
				"filename": "Dockerfile",
			},
			LocalMounts: map[string]fsutil.FS{
				"context":    ctxMount,
				"dockerfile": ctxMount,
			},
			Exports: []bkclient.ExportEntry{{
				Type:  bkclient.ExporterDocker,
				Attrs: map[string]string{"name": imageTag},
				Output: func(_ map[string]string) (io.WriteCloser, error) {
					return pipeW, nil
				},
			}},
		}, ch)
		return err
	})

	eg.Go(func() error {
		var buildOut io.Writer
		if b.workDir != nil {
			logFile, logErr := b.workDir.CreateBuildLog(serverName, cacheKey)
			if logErr == nil {
				defer func() { _ = logFile.Close() }()
				buildOut = logFile
			}
		}
		for status := range ch {
			if buildOut != nil {
				for _, log := range status.Logs {
					_, _ = buildOut.Write(log.Data)
				}
			}
		}
		return nil
	})

	eg.Go(func() error {
		resp, err := b.docker.ImageLoad(ctx, pipeR)
		if err != nil {
			return fmt.Errorf("loading image into docker: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}

type fetchResult struct {
	data []byte
	hash string
}

func (b *Builder) fetchBinary(ctx context.Context, srv *spec.ServerSpec, artifact *resolver.Artifact) ([]byte, bool, error) {
	if !b.opts.NoCache && b.workDir != nil && artifact.SHA256 != "" && b.workDir.HasBinary(artifact.SHA256) {
		b.logger.Info("using cached binary", "server", srv.Name, "sha256", artifact.SHA256[:12])
		data, err := b.workDir.ReadBinary(artifact.SHA256)
		if err == nil {
			return data, true, nil
		}
		b.logger.Warn("failed to read cached binary, re-downloading", "error", err)
	}

	b.logger.Info("downloading binary", "server", srv.Name, "url", artifact.URL)
	result, err, _ := b.fetchOnce.Do(artifact.URL, func() (any, error) {
		data, dlErr := resolver.GetRaw(ctx, artifact.URL)
		if dlErr != nil {
			return nil, fmt.Errorf("downloading binary: %w", dlErr)
		}

		h := sha256.Sum256(data)
		actualHash := hex.EncodeToString(h[:])

		if artifact.SHA256 != "" && actualHash != artifact.SHA256 {
			return nil, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", srv.Software, artifact.SHA256, actualHash)
		}

		if b.workDir != nil {
			if storeErr := b.workDir.StoreBinary(actualHash, data, srv.Software, artifact.URL); storeErr != nil {
				b.logger.Warn("failed to cache binary", "error", storeErr)
			}
		}

		return &fetchResult{data: data, hash: actualHash}, nil
	})
	if err != nil {
		return nil, false, err
	}

	fr := result.(*fetchResult)
	if artifact.SHA256 == "" {
		artifact.SHA256 = fr.hash
	}

	return fr.data, false, nil
}

func (b *Builder) platform(ctx context.Context) (resolver.Platform, error) {
	ver, err := b.docker.ServerVersion(ctx)
	if err != nil {
		return resolver.Platform{}, err
	}
	return resolver.Platform{OS: ver.Os, Arch: ver.Arch}, nil
}

func (b *Builder) imageExists(ctx context.Context, tag string) bool {
	images, err := b.docker.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", tag)),
	})
	if err != nil {
		return false
	}
	return len(images) > 0
}
