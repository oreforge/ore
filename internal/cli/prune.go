package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/spec"
)

func newPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune <target>",
		Short: "Remove unused resources (all, containers, images, volumes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := map[string]project.PruneTarget{
				"all":        project.PruneAll,
				"containers": project.PruneContainers,
				"images":     project.PruneImages,
				"volumes":    project.PruneVolumes,
			}
			target, ok := targets[args[0]]
			if !ok {
				return fmt.Errorf("unknown prune target %q (use: all, containers, images, volumes)", args[0])
			}

			if localMode {
				s, err := spec.Load(specPath)
				if err != nil {
					return err
				}

				dockerClient, err := docker.New(cmd.Context())
				if err != nil {
					return fmt.Errorf("connecting to Docker: %w", err)
				}
				defer func() { _ = dockerClient.Close() }()

				deployer := deploy.New(dockerClient, logger, nil, true)
				return project.ExecutePrune(cmd.Context(), deployer, s, filepath.Dir(specPath), target, logger)
			}
			return remoteClient.Prune(cmd.Context(), target)
		},
	}
}
