package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/spec"
)

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove servers, images, data, or build cache",
	}

	cmd.AddCommand(
		newCleanAllCmd(),
		newCleanCacheCmd(),
		newCleanBuildsCmd(),
		newCleanServersCmd(),
		newCleanImagesCmd(),
		newCleanDataCmd(),
	)

	return cmd
}

func newCleanAllCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "all",
		Short: "Remove all servers, images, data, and build cache",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				return pruneLocal(cmd, project.PruneAll)
			}
			return remoteClient.Prune(cmd.Context(), project.PruneAll)
		},
	}
}

func newCleanCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cache",
		Short: "Remove cached binaries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				return cleanLocalDir("cache")
			}
			return remoteClient.Clean(cmd.Context(), project.CleanCache)
		},
	}
}

func newCleanBuildsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "builds",
		Short: "Remove build artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				return cleanLocalDir("builds")
			}
			return remoteClient.Clean(cmd.Context(), project.CleanBuilds)
		},
	}
}

func newCleanServersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "servers",
		Short: "Stop and remove all running servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				return pruneLocal(cmd, project.PruneContainers)
			}
			return remoteClient.Prune(cmd.Context(), project.PruneContainers)
		},
	}
}

func newCleanImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "Remove unused server images",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				return pruneLocal(cmd, project.PruneImages)
			}
			return remoteClient.Prune(cmd.Context(), project.PruneImages)
		},
	}
}

func newCleanDataCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "data",
		Short: "Remove server data volumes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				return pruneLocal(cmd, project.PruneVolumes)
			}
			return remoteClient.Prune(cmd.Context(), project.PruneVolumes)
		},
	}
}

func cleanLocalDir(target string) error {
	repoRoot := filepath.Dir(specPath)
	wd, err := build.NewWorkDir(repoRoot, logger)
	if err != nil {
		return fmt.Errorf("opening .ore directory: %w", err)
	}
	if target == "cache" {
		return wd.CleanCache()
	}
	return wd.CleanBuilds()
}

func pruneLocal(cmd *cobra.Command, target project.PruneTarget) error {
	s, err := spec.Load(specPath)
	if err != nil {
		return err
	}

	dockerClient, err := docker.New(cmd.Context())
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	deployer := deploy.New(dockerClient, logger, nil, true)
	return project.ExecutePrune(cmd.Context(), deployer, s, filepath.Dir(specPath), target, logger)
}
