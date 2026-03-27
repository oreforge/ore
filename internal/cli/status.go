package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/orchestrator"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the status of all servers in the network",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")

			status, err := eng.Status(cmd.Context(), configFile)
			if err != nil {
				return err
			}

			if jsonOut {
				return printJSON(status)
			}
			return printTable(status)
		},
	}

	cmd.Flags().Bool("json", false, "output as JSON")

	return cmd
}

func printJSON(status *orchestrator.NetworkStatus) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func printTable(status *orchestrator.NetworkStatus) error {
	fmt.Printf("Network: %s\n\n", status.Network)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "SERVER\tSTATUS\tHEALTH\tIMAGE\tPORTS\tUPTIME\tRESTARTS")

	for _, srv := range status.Servers {
		for _, c := range srv.Replicas {
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
				c.State,
				c.Health,
				image,
				ports,
				uptime,
				c.RestartCount,
			)
		}
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
