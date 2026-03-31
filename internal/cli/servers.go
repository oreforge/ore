package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/config"
)

func newServersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "servers",
		Short: "Manage remote server connections",
	}

	cmd.AddCommand(
		newServersListCmd(),
		newServersAddCmd(),
		newServersRemoveCmd(),
		newServersUseCmd(),
		newServersActiveCmd(),
		newServersShowCmd(),
	)

	return cmd
}

func newServersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List configured servers",
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			if len(cfg.Servers) == 0 {
				fmt.Println("no servers configured")
				return nil
			}

			for _, name := range slices.Sorted(maps.Keys(cfg.Servers)) {
				if name == cfg.Context {
					fmt.Printf("* %s\n", name)
				} else {
					fmt.Printf("  %s\n", name)
				}
			}
			return nil
		},
	}
}

func newServersAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "add <name>",
		Short:       "Add or update a remote server",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("addr")
			token, _ := cmd.Flags().GetString("token")
			project, _ := cmd.Flags().GetString("project")

			if err := config.SaveServer(args[0], config.ServerConfig{
				Addr:    addr,
				Token:   token,
				Project: project,
			}); err != nil {
				return err
			}

			fmt.Printf("added server %q\n", args[0])
			return nil
		},
	}

	cmd.Flags().String("addr", "", "server address (host:port)")
	cmd.Flags().String("token", "", "authentication token")
	cmd.Flags().String("project", "", "default project (optional)")

	_ = cmd.MarkFlagRequired("addr")
	_ = cmd.MarkFlagRequired("token")

	return cmd
}

func newServersRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "remove <name>",
		Short:       "Remove a server",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			if err := config.RemoveServer(args[0]); err != nil {
				return err
			}

			fmt.Printf("removed server %q\n", args[0])
			return nil
		},
	}
}

func newServersUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Short:       "Set the active server",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			if _, exists := cfg.Servers[args[0]]; !exists {
				return fmt.Errorf("server %q not found", args[0])
			}

			if err := config.SetContext(args[0]); err != nil {
				return err
			}

			fmt.Printf("switched to server %q\n", args[0])
			return nil
		},
	}
}

func newServersActiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "active",
		Short:       "Show the active server",
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			if cfg.Context == "" {
				fmt.Println("no active server")
			} else {
				fmt.Println(cfg.Context)
			}
			return nil
		},
	}
}

func newServersShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "show <name>",
		Short:       "Show server details",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			srv, exists := cfg.Servers[args[0]]
			if !exists {
				return fmt.Errorf("server %q not found", args[0])
			}

			fmt.Printf("Name:    %s\n", args[0])
			fmt.Printf("Addr:    %s\n", srv.Addr)
			fmt.Printf("Token:   %s\n", maskToken(srv.Token))
			if srv.Project != "" {
				fmt.Printf("Project: %s\n", srv.Project)
			}
			return nil
		},
	}
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
