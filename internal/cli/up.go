package cli

import (
	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/project"
)

func newUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Build and start all servers",
		Example: `ore up
ore up --no-cache --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			noCache, _ := cmd.Flags().GetBool("no-cache")
			force, _ := cmd.Flags().GetBool("force")

			local, specPath, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}

			if local {
				be, cleanup, err := newBuildEnv(cmd.Context(), specPath, build.Options{NoCache: noCache})
				if err != nil {
					return err
				}
				defer cleanup()

				images, err := be.buildAll(cmd.Context())
				if err != nil {
					return err
				}

				var prevState *deploy.State
				if !force && be.workDir != nil {
					prevState = deploy.LoadState(be.workDir.Root())
				}

				deployer := deploy.New(be.docker, logger, be.workDir, true)
				newState, err := deployer.Up(cmd.Context(), be.spec, images, deploy.UpOptions{
					PrevState: prevState,
					Force:     force,
				})
				if err != nil {
					return err
				}

				if be.workDir != nil && newState != nil {
					if saveErr := deploy.SaveState(be.workDir.Root(), newState); saveErr != nil {
						logger.Warn("failed to save deploy state", "error", saveErr)
					}
				}

				return nil
			}
			return remote.Up(cmd.Context(), project.UpOptions{
				NoCache: noCache,
				Force:   force,
			})
		},
	}

	cmd.Flags().Bool("no-cache", false, "skip local binary cache and re-download everything")
	cmd.Flags().Bool("force", false, "force restart all servers even if unchanged")

	return cmd
}
