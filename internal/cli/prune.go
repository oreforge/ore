package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/client"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

type pruneTarget int

const (
	pruneAll pruneTarget = iota
	pruneContainers
	pruneImages
	pruneVolumes
)

func newPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune <target>",
		Short: "Remove unused resources (all, containers, images, volumes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if localMode {
				targets := map[string]pruneTarget{
					"all":        pruneAll,
					"containers": pruneContainers,
					"images":     pruneImages,
					"volumes":    pruneVolumes,
				}
				target, ok := targets[args[0]]
				if !ok {
					return fmt.Errorf("unknown prune target %q (use: all, containers, images, volumes)", args[0])
				}

				s, err := spec.Load(specPath)
				if err != nil {
					return err
				}

				dockerClient, err := docker.New(cmd.Context())
				if err != nil {
					return fmt.Errorf("connecting to Docker: %w", err)
				}
				defer func() { _ = dockerClient.Close() }()

				orch := deploy.New(dockerClient, logger, nil, true)

				switch target {
				case pruneAll:
					var errs []error
					if err := orch.Down(cmd.Context(), s); err != nil {
						errs = append(errs, fmt.Errorf("stopping containers: %w", err))
					}
					if err := orch.PruneImages(cmd.Context(), s); err != nil {
						errs = append(errs, fmt.Errorf("pruning images: %w", err))
					}
					if err := orch.PruneVolumes(cmd.Context(), s); err != nil {
						errs = append(errs, fmt.Errorf("pruning volumes: %w", err))
					}
					repoRoot := filepath.Dir(specPath)
					if wd, wdErr := build.NewWorkDir(repoRoot, logger); wdErr == nil {
						if cleanErr := wd.Clean(); cleanErr != nil {
							errs = append(errs, fmt.Errorf("cleaning .ore directory: %w", cleanErr))
						}
					}
					logger.Info("pruned all resources")
					return errors.Join(errs...)
				case pruneContainers:
					return orch.Down(cmd.Context(), s)
				case pruneImages:
					return orch.PruneImages(cmd.Context(), s)
				case pruneVolumes:
					return orch.PruneVolumes(cmd.Context(), s)
				default:
					return fmt.Errorf("unknown prune target: %d", target)
				}
			}
			targets := map[string]client.PruneTarget{
				"all":        client.PruneAll,
				"containers": client.PruneContainers,
				"images":     client.PruneImages,
				"volumes":    client.PruneVolumes,
			}
			target, ok := targets[args[0]]
			if !ok {
				return fmt.Errorf("unknown prune target %q (use: all, containers, images, volumes)", args[0])
			}
			return remoteClient.Prune(cmd.Context(), target)
		},
	}
}
