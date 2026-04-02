package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/software/providers"
	"github.com/oreforge/ore/internal/spec"
)

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build Docker images for all servers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			if localMode {
				s, err := spec.Load(specPath)
				if err != nil {
					return err
				}

				dockerClient, err := docker.New(cmd.Context())
				if err != nil {
					return fmt.Errorf("connecting to Docker: %w", err)
				}
				defer func() { _ = dockerClient.Close() }()

				bk, err := docker.NewBuildKitClient(cmd.Context(), dockerClient)
				if err != nil {
					return fmt.Errorf("connecting to BuildKit: %w", err)
				}
				defer func() { _ = bk.Close() }()

				repoRoot := filepath.Dir(specPath)
				wd, err := build.NewWorkDir(repoRoot, logger)
				if err != nil {
					return fmt.Errorf("initializing .ore directory: %w", err)
				}

				builder := build.NewBuilder(dockerClient, bk, providers.New(), logger, wd, build.Options{NoCache: noCache, ForceBuild: true})
				images, err := builder.BuildAll(cmd.Context(), s, repoRoot)
				if err != nil {
					return err
				}

				for name, res := range images {
					logger.Info("built image", "server", name, "tag", res.ImageTag)
				}

				return nil
			}
			return remoteClient.Build(cmd.Context(), noCache)
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")

	return cmd
}
