package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/client"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/software/providers"
	"github.com/oreforge/ore/internal/spec"
)

func newUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build images and start the network",
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			force, _ := cmd.Flags().GetBool("force")
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

				bk, err := docker.NewBuildKitClient(cmd.Context(), dockerClient)
				if err != nil {
					return fmt.Errorf("connecting to BuildKit: %w", err)
				}
				defer func() { _ = bk.Close() }()

				repoRoot := filepath.Dir(specPath)
				wd, err := build.NewWorkDir(repoRoot, logger)
				if err != nil {
					return fmt.Errorf("initializing .ore directory: %w", err)
				}

				builder := build.NewBuilder(dockerClient, bk, providers.New(), logger, wd, build.Options{NoCache: noCache})
				images, err := builder.BuildAll(cmd.Context(), s, repoRoot)
				if err != nil {
					return err
				}

				for name, res := range images {
					logger.Info("built image", "server", name, "tag", res.ImageTag)
				}

				var prevState *deploy.DeployState
				if !force && wd != nil {
					prevState = deploy.LoadState(wd.Root())
				}

				orch := deploy.New(dockerClient, logger, wd, true)
				newState, err := orch.Up(cmd.Context(), s, images, deploy.UpOptions{
					PrevState: prevState,
					Force:     force,
				})
				if err != nil {
					return err
				}

				if wd != nil && newState != nil {
					if saveErr := deploy.SaveState(wd.Root(), newState); saveErr != nil {
						logger.Warn("failed to save deploy state", "error", saveErr)
					}
				}

				return nil
			}
			return remoteClient.Up(cmd.Context(), client.UpOptions{
				NoCache: noCache,
				Force:   force,
			})
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")
	cmd.Flags().Bool("force", false, "force restart all containers even if unchanged")

	return cmd
}
