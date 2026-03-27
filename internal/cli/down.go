package cli

import (
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop all containers and remove the network",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return eng.Down(cmd.Context(), configFile)
		},
	}
}
