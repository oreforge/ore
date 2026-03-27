package cli

import (
	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/engine"
)

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove the .ore/ cache and build artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			target := engine.CleanAll
			if cacheOnly, _ := cmd.Flags().GetBool("cache"); cacheOnly {
				target = engine.CleanCache
			} else if buildsOnly, _ := cmd.Flags().GetBool("builds"); buildsOnly {
				target = engine.CleanBuilds
			}

			return eng.Clean(cmd.Context(), configFile, target)
		},
	}

	cmd.Flags().Bool("cache", false, "only remove cached binaries")
	cmd.Flags().Bool("builds", false, "only remove build artifacts")

	return cmd
}
