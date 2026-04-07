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
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	dockerbuild "github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/docker/docker/pkg/jsonmessage"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/software"
	"github.com/oreforge/ore/internal/spec"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

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
	resolver  *software.Resolver
	logger    *slog.Logger
	workDir   *WorkDir
	opts      Options
	fetchOnce singleflight.Group
}

func NewBuilder(dockerClient docker.Client, resolver *software.Resolver, logger *slog.Logger, workDir *WorkDir, opts Options) *Builder {
	return &Builder{
		docker:   dockerClient,
		resolver: resolver,
		logger:   logger,
		workDir:  workDir,
		opts:     opts,
	}
}

func (b *Builder) BuildAll(ctx context.Context, cfg *spec.Network, repoRoot string) (map[string]Result, error) {
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

func (b *Builder) Build(ctx context.Context, srv *spec.Server, repoRoot string) (Result, error) {
	startedAt := time.Now()

	b.logger.Debug("resolving software", "server", srv.Name, "software", srv.Software)

	artifact, err := b.resolver.Resolve(ctx, srv.Software)
	if err != nil {
		return Result{}, fmt.Errorf("resolving software: %w", err)
	}

	serverDir := filepath.Join(repoRoot, srv.Dir)

	cacheKey, err := CacheKey(srv.Software, artifact.Version, serverDir)
	if err != nil {
		return Result{}, fmt.Errorf("computing cache key: %w", err)
	}

	imageTag := fmt.Sprintf("ore/%s:%s", srv.Name, cacheKey)
	resolvedHC := resolveServerHealthCheck(srv.HealthCheck, artifact.Health)
	result := Result{ImageTag: imageTag, HealthTimeout: resolvedHC.WaitTimeout()}

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
	dfOpts := DockerfileOptions{
		Runtime:     artifact.Runtime,
		ExtraArgs:   artifact.Runtime.ExtraArgs,
		HealthCheck: resolvedHC,
	}
	dockerfile := GenerateDockerfile(dfOpts)
	entrypoint := artifact.Runtime.Entrypoint
	binaryName := artifact.Runtime.BinaryName
	binaryMode := os.FileMode(artifact.Runtime.BinaryMode)

	buildDir, cleanup, err := b.prepareBuildDir(srv.Name, cacheKey, serverDir, dockerfile, entrypoint, binaryName, binaryData, binaryMode)
	if err != nil {
		return Result{}, fmt.Errorf("preparing build context: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	b.logger.Info("building image", "server", srv.Name, "tag", imageTag)

	if err := b.buildImage(ctx, buildDir, imageTag, srv.Name, cacheKey); err != nil {
		return Result{}, err
	}

	duration := time.Since(startedAt)
	b.logger.Info("image built", "server", srv.Name, "tag", imageTag, "duration_ms", duration.Milliseconds())

	if b.workDir != nil {
		meta := Metadata{
			ServerName:   srv.Name,
			SoftwareID:   srv.Software,
			ArtifactURL:  artifact.URL,
			ImageTag:     imageTag,
			CacheKey:     cacheKey,
			Runtime:      srv.Software,
			BinaryCached: cached,
			StartedAt:    startedAt,
			DurationMs:   duration.Milliseconds(),
		}
		if err := b.workDir.WriteMetadata(srv.Name, cacheKey, meta); err != nil {
			b.logger.Warn("failed to write build metadata", "server", srv.Name, "error", err)
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
	if err := copyDir(serverDir, filepath.Join(tmpDir, "data")); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmpDir, cleanup, nil
}

func (b *Builder) buildImage(ctx context.Context, buildDir, imageTag, serverName, cacheKey string) error {
	buildContext, err := createTarContext(buildDir)
	if err != nil {
		return fmt.Errorf("creating build context: %w", err)
	}

	resp, err := b.docker.ImageBuild(ctx, buildContext, dockerbuild.ImageBuildOptions{
		Tags:        []string{imageTag},
		Dockerfile:  "Dockerfile",
		Remove:      true,
		ForceRemove: true,
		Version:     dockerbuild.BuilderBuildKit,
	})
	if err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	buildOut := io.Discard
	if b.workDir != nil {
		logFile, logErr := b.workDir.CreateBuildLog(serverName, cacheKey)
		if logErr == nil {
			defer func() { _ = logFile.Close() }()
			buildOut = logFile
		}
	}

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, buildOut, 0, false, nil); err != nil {
		return fmt.Errorf("docker build for %s failed: %w", serverName, err)
	}

	return nil
}

func createTarContext(dir string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

type fetchResult struct {
	data []byte
	hash string
}

func (b *Builder) fetchBinary(ctx context.Context, srv *spec.Server, artifact *software.Artifact) ([]byte, bool, error) {
	if !b.opts.NoCache && b.workDir != nil && artifact.SHA256 != "" && b.workDir.HasBinary(artifact.SHA256) {
		b.logger.Info("using cached binary", "server", srv.Name, "sha256", artifact.SHA256[:12])
		data, err := b.workDir.ReadBinary(artifact.SHA256)
		if err == nil {
			return data, true, nil
		}
		b.logger.Warn("failed to read cached binary, re-downloading", "server", srv.Name, "error", err)
	}

	b.logger.Info("downloading binary", "server", srv.Name, "url", artifact.URL)
	result, err, _ := b.fetchOnce.Do(artifact.URL, func() (any, error) {
		data, dlErr := httpGet(ctx, artifact.URL)
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
				b.logger.Warn("failed to cache binary", "server", srv.Name, "error", storeErr)
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

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oreforge/ore")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
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

func resolveServerHealthCheck(user *spec.HealthCheck, provider software.HealthCheck) *spec.HealthCheck {
	if user != nil && user.Disabled {
		return &spec.HealthCheck{Disabled: true}
	}

	resolved := &spec.HealthCheck{
		Cmd:         provider.Cmd,
		Interval:    provider.Interval,
		Timeout:     provider.Timeout,
		StartPeriod: provider.StartPeriod,
		Retries:     provider.Retries,
	}

	if user == nil {
		return resolved
	}

	if user.Cmd != "" {
		resolved.Cmd = user.Cmd
	}
	if user.Interval != 0 {
		resolved.Interval = user.Interval
	}
	if user.Timeout != 0 {
		resolved.Timeout = user.Timeout
	}
	if user.StartPeriod != 0 {
		resolved.StartPeriod = user.StartPeriod
	}
	if user.Retries != 0 {
		resolved.Retries = user.Retries
	}

	return resolved
}
