package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "down",
		Short:   "Stop and remove all servers",
		Example: "ore down",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
				return deployer.Down(cmd.Context(), s)
			}
			return remoteClient.Down(cmd.Context())
		},
	}
}
