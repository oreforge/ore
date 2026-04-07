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

func newBuildEnv(ctx context.Context, specPath string, opts build.Options) (*buildEnv, func(), error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return nil, nil, err
	}

	dockerClient, err := docker.New(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	repoRoot := filepath.Dir(specPath)
	wd, err := build.NewWorkDir(repoRoot, logger)
	if err != nil {
		_ = dockerClient.Close()
		return nil, nil, fmt.Errorf("initializing .ore directory: %w", err)
	}

	builder := build.NewBuilder(dockerClient, providers.New(), logger, wd, opts)

	cleanup := func() {
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
		Short: "Build all server images",
		Example: `ore build
ore build --no-cache`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")

			local, specPath, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}

			if local {
				be, cleanup, err := newBuildEnv(cmd.Context(), specPath, build.Options{NoCache: noCache, ForceBuild: true})
				if err != nil {
					return err
				}
				defer cleanup()

				_, err = be.buildAll(cmd.Context())
				return err
			}
			return remote.Build(cmd.Context(), noCache)
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")

	return cmd
}
