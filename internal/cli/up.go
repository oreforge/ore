package cli

import (
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build images and start the network",
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			return eng.Up(cmd.Context(), noCache)
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")

	return cmd
}
