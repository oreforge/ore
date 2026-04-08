package cli

import (
	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/project"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "down",
		Short:   "Stop and remove all containers",
		Example: "ore down",
		Args:    cobra.NoArgs,
		RunE:    cleanRunE(project.CleanContainers),
	}
}
