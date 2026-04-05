package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/software/providers"
	"github.com/oreforge/ore/internal/spec"
)

type buildEnv struct {
	spec     *spec.Network
	docker   docker.Client
	workDir  *build.WorkDir
	builder  *build.Builder
	repoRoot string
}

func newBuildEnv(ctx context.Context, opts build.Options) (*buildEnv, func(), error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	bk, err := docker.NewBuildKitClient(ctx, dockerClient)
	if err != nil {
		_ = dockerClient.Close()
		return nil, nil, fmt.Errorf("connecting to BuildKit: %w", err)
	}

	repoRoot := filepath.Dir(specPath)
	wd, err := build.NewWorkDir(repoRoot, logger)
	if err != nil {
		_ = bk.Close()
		_ = dockerClient.Close()
		return nil, nil, fmt.Errorf("initializing .ore directory: %w", err)
	}

	builder := build.NewBuilder(dockerClient, bk, providers.New(), logger, wd, opts)

	cleanup := func() {
		_ = bk.Close()
		_ = dockerClient.Close()
	}

	return &buildEnv{
		spec:     s,
		docker:   dockerClient,
		workDir:  wd,
		builder:  builder,
		repoRoot: repoRoot,
	}, cleanup, nil
}

func (be *buildEnv) buildAll(ctx context.Context) (map[string]build.Result, error) {
	images, err := be.builder.BuildAll(ctx, be.spec, be.repoRoot)
	if err != nil {
		return nil, err
	}

	for name, res := range images {
		logger.Info("built image", "server", name, "tag", res.ImageTag)
	}

	return images, nil
}

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build Docker images for all servers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			if localMode {
				be, cleanup, err := newBuildEnv(cmd.Context(), build.Options{NoCache: noCache, ForceBuild: true})
				if err != nil {
					return err
				}
				defer cleanup()

				_, err = be.buildAll(cmd.Context())
				return err
			}
			return remoteClient.Build(cmd.Context(), noCache)
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")

	return cmd
}
