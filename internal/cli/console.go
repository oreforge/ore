package cli

import (
	"fmt"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/client"
	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/console"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

func completeServerNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	specFile, _ := cmd.Flags().GetString("file")
	if _, err := os.Stat(specFile); err == nil {
		s, err := spec.Load(specFile)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		names := make([]string, len(s.Servers))
		for i, srv := range s.Servers {
			names[i] = srv.Name
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}

	addr, token, project, ok := config.ResolveRemote(cfg)
	if !ok || project == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	r, err := client.New(addr, token, project)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer func() { _ = r.Close() }()

	status, err := r.Status(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, len(status.Servers))
	for i, srv := range status.Servers {
		names[i] = srv.Name
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func newConsoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "console <server>",
		Short:             "Attach to a running server console",
		Example:           "ore console survival",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeServerNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}

			if local {
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
			return remote.Console(cmd.Context(), args[0])
		},
	}
}
