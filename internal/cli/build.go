package cli

import (
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build Docker images for all servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			return eng.Build(cmd.Context(), configFile, noCache)
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")

	return cmd
}
