package build

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	dockerbuild "github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
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
}

type Builder struct {
	docker    docker.Client
	registry  *providers.Registry
	logger    *slog.Logger
	workDir   *cache.Manager
	opts      Options
	fetchOnce singleflight.Group
}

func NewBuilder(dockerClient docker.Client, registry *providers.Registry, logger *slog.Logger, workDir *cache.Manager, opts Options) *Builder {
	return &Builder{
		docker:   dockerClient,
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

	if b.workDir != nil {
		b.workDir.CleanOldBuilds(srv.Name, cacheKey)
		if err := b.workDir.WriteDockerfile(srv.Name, cacheKey, dockerfile); err != nil {
			b.logger.Warn("failed to write Dockerfile artifact", "error", err)
		}
		if err := b.workDir.WriteDataDir(srv.Name, cacheKey, serverDir); err != nil {
			b.logger.Warn("failed to write data dir artifact", "error", err)
		}
	}

	b.logger.Info("building image", "server", srv.Name, "tag", imageTag)

	binaryName := artifact.Runtime.BinaryName()

	buildCtx, err := createBuildContext(binaryData, binaryName, artifact.Runtime.BinaryMode(), serverDir, dockerfile, entrypoint)
	if err != nil {
		return Result{}, fmt.Errorf("creating build context: %w", err)
	}

	resp, err := b.docker.ImageBuild(ctx, buildCtx, dockerbuild.ImageBuildOptions{
		Tags:        []string{imageTag},
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return Result{}, fmt.Errorf("docker build: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var buildOut io.Writer
	if b.workDir != nil {
		logFile, logErr := b.workDir.CreateBuildLog(srv.Name, cacheKey)
		if logErr == nil {
			defer func() { _ = logFile.Close() }()
			buildOut = logFile
		}
	}
	if buildOut == nil {
		buildOut = io.Discard
	}

	if _, err := io.Copy(buildOut, resp.Body); err != nil {
		return Result{}, fmt.Errorf("reading build output: %w", err)
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

func createBuildContext(binaryData []byte, binaryName string, binaryMode int64, serverDir, dockerfile, entrypoint string) (io.Reader, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	if err := addToTar(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}

	if err := addToTar(tw, binaryName, binaryData, binaryMode); err != nil {
		return nil, err
	}

	if entrypoint != "" {
		if err := addToTar(tw, "entrypoint.sh", []byte(entrypoint), 0o755); err != nil {
			return nil, err
		}
	}

	if err := addDirectoryToTar(tw, serverDir, "data"); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return buf, nil
}

func addToTar(tw *tar.Writer, name string, data []byte, mode int64) error {
	header := &tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: mode,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func addDirectoryToTar(tw *tar.Writer, srcDir, prefix string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		tarPath := filepath.Join(prefix, rel)

		if info.IsDir() {
			return tw.WriteHeader(&tar.Header{
				Name:     tarPath + "/",
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			})
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return addToTar(tw, tarPath, data, 0o644)
	})
}
