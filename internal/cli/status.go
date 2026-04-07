package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show server and service status",
		Example: "ore status",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			local, specPath, remote, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			if remote != nil {
				defer func() { _ = remote.Close() }()
			}

			var status *deploy.NetworkStatus
			if local {
				s, loadErr := spec.Load(specPath)
				if loadErr != nil {
					return loadErr
				}

				dockerClient, dockerErr := docker.New(cmd.Context())
				if dockerErr != nil {
					return fmt.Errorf("connecting to Docker: %w", dockerErr)
				}
				defer func() { _ = dockerClient.Close() }()

				deployer := deploy.New(dockerClient, logger, nil, true)
				status, err = deployer.Status(cmd.Context(), s)
			} else {
				status, err = remote.Status(cmd.Context())
			}
			if err != nil {
				return err
			}

			return printTable(status)
		},
	}

	return cmd
}

func printTable(status *deploy.NetworkStatus) error {
	fmt.Printf("Network: %s\n\n", status.Network)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATUS\tHEALTH\tIMAGE\tPORTS\tUPTIME\tRESTARTS")

	for _, srv := range status.Servers {
		c := srv.Container
		ports := "—"
		if len(c.Ports) > 0 {
			portStrs := make([]string, len(c.Ports))
			for i, p := range c.Ports {
				portStrs[i] = p.String()
			}
			ports = strings.Join(portStrs, ", ")
		}

		uptime := "—"
		if c.Uptime > 0 {
			uptime = formatDuration(c.Uptime)
		}

		image := c.Image
		if image == "" {
			image = "—"
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			c.Name,
			c.State.String(),
			c.Health.String(),
			image,
			ports,
			uptime,
			c.RestartCount,
		)
	}

	return w.Flush()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
