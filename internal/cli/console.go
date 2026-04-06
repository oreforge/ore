package cli

import (
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/console"
	"github.com/oreforge/ore/internal/docker"
)

func newConsoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "console <server>",
		Short:   "Attach to a running server console",
		Example: "ore console survival",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if localMode {
				dockerClient, err := docker.New(cmd.Context())
				if err != nil {
					return fmt.Errorf("connecting to Docker: %w", err)
				}
				defer func() { _ = dockerClient.Close() }()

				hijacked, err := dockerClient.ContainerAttach(cmd.Context(), args[0], container.AttachOptions{
					Stream: true,
					Stdin:  true,
					Stdout: true,
					Stderr: true,
					Logs:   true,
				})
				if err != nil {
					return fmt.Errorf("attaching to server %s: %w", args[0], err)
				}
				defer hijacked.Close()

				return console.Run(cmd.Context(), console.NewDockerConn(hijacked, dockerClient, args[0]))
			}
			return remoteClient.Console(cmd.Context(), args[0])
		},
	}
}
