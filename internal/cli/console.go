package cli

import (
	"github.com/spf13/cobra"
)

func newConsoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "console <server>",
		Short: "Attach to a running server console",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return eng.Console(cmd.Context(), args[0])
		},
	}
}
