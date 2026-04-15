package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/project"
)

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove containers, images, volumes, or build artifacts",
	}

	cmd.AddCommand(
		newCleanAllCmd(),
		newCleanContainersCmd(),
		newCleanImagesCmd(),
		newCleanVolumesCmd(),
		newCleanCacheCmd(),
		newCleanBuildsCmd(),
	)

	return cmd
}

func newCleanAllCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "all",
		Short:   "Remove all containers, images, volumes, and build artifacts",
		Example: `ore clean all`,
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanAll),
	}
}

func newCleanContainersCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "containers",
		Short:   "Stop and remove all containers and the network",
		Example: `ore clean containers`,
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanContainers),
	}
}

func newCleanImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "images",
		Short:   "Remove Docker images built by ore",
		Example: `ore clean images`,
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanImages),
	}
}

func newCleanVolumesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "volumes",
		Short:   "Remove Docker volumes (persistent server data)",
		Example: `ore clean volumes`,
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanVolumes),
	}
}

func newCleanCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "cache",
		Short:   "Remove cached software binaries",
		Example: `ore clean cache`,
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanCache),
	}
}

func newCleanBuildsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "builds",
		Short:   "Remove build artifacts",
		Example: `ore clean builds`,
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanBuilds),
	}
}

func cleanRunE(target project.CleanTarget) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		local, specPath, remote, err := resolveMode(cmd)
		if err != nil {
			return err
		}
		if remote != nil {
			defer func() { _ = remote.Close() }()
		}

		if local {
			return project.ExecuteClean(cmd.Context(), specPath, filepath.Dir(specPath), target, true, logger)
		}
		return remote.Clean(cmd.Context(), target)
	}
}
