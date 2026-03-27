package cli

import (
	"github.com/spf13/cobra"
)

func newConsoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console <server>",
		Short: "Attach to a running server console",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			replica, _ := cmd.Flags().GetInt("replica")
			return eng.Console(cmd.Context(), args[0], replica)
		},
	}

	cmd.Flags().IntP("replica", "r", 1, "replica number for replicated servers")

	return cmd
}
