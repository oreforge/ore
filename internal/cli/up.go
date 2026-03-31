package cli

import (
	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/engine"
)

func newUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build images and start the network",
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			force, _ := cmd.Flags().GetBool("force")
			return eng.Up(cmd.Context(), engine.UpOptions{
				NoCache: noCache,
				Force:   force,
			})
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")
	cmd.Flags().Bool("force", false, "force restart all containers even if unchanged")

	return cmd
}
