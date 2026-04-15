package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the ore version",
		Example: `ore version`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("ore %s (%s/%s)\n", info.Version, runtime.GOOS, runtime.GOARCH)
			if info.Commit != "" {
				fmt.Printf("commit: %s\n", info.Commit)
			}
			if info.BuildDate != "" {
				fmt.Printf("built: %s\n", info.BuildDate)
			}
			return nil
		},
	}
}
