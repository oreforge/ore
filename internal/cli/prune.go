package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/engine"
)

func newPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune <target>",
		Short: "Remove unused resources (all, containers, images, volumes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := map[string]engine.PruneTarget{
				"all":        engine.PruneAll,
				"containers": engine.PruneContainers,
				"images":     engine.PruneImages,
				"volumes":    engine.PruneVolumes,
			}

			target, ok := targets[args[0]]
			if !ok {
				return fmt.Errorf("unknown prune target %q (use: all, containers, images, volumes)", args[0])
			}

			return eng.Prune(cmd.Context(), target)
		},
	}
}
