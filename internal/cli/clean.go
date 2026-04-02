package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/client"
)

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove the .ore/ cache and build artifacts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localMode {
				repoRoot := filepath.Dir(specPath)
				wd, err := build.NewWorkDir(repoRoot, logger)
				if err != nil {
					return fmt.Errorf("opening .ore directory: %w", err)
				}

				if cacheOnly, _ := cmd.Flags().GetBool("cache"); cacheOnly {
					return wd.CleanCache()
				}
				if buildsOnly, _ := cmd.Flags().GetBool("builds"); buildsOnly {
					return wd.CleanBuilds()
				}
				return wd.Clean()
			}
			target := client.CleanAll
			if cacheOnly, _ := cmd.Flags().GetBool("cache"); cacheOnly {
				target = client.CleanCache
			} else if buildsOnly, _ := cmd.Flags().GetBool("builds"); buildsOnly {
				target = client.CleanBuilds
			}
			return remoteClient.Clean(cmd.Context(), target)
		},
	}

	cmd.Flags().Bool("cache", false, "only remove cached binaries")
	cmd.Flags().Bool("builds", false, "only remove build artifacts")

	return cmd
}
