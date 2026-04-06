package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/config"
)

func newNodesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nodes",
		Short: "Manage remote node connections",
	}

	cmd.AddCommand(
		newNodesListCmd(),
		newNodesAddCmd(),
		newNodesRemoveCmd(),
		newNodesUseCmd(),
		newNodesActiveCmd(),
		newNodesShowCmd(),
	)

	return cmd
}

func newNodesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List configured nodes",
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			if len(cfg.Nodes) == 0 {
				fmt.Println("no nodes configured")
				return nil
			}

			for _, name := range slices.Sorted(maps.Keys(cfg.Nodes)) {
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

func newNodesAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "add <name>",
		Short:       "Add or update a remote node",
		Example:     "ore nodes add mynode --addr 192.168.1.10:9090 --token mytoken",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("addr")
			token, _ := cmd.Flags().GetString("token")
			project, _ := cmd.Flags().GetString("project")

			if err := config.SaveNode(args[0], config.NodeConfig{
				Addr:    addr,
				Token:   token,
				Project: project,
			}); err != nil {
				return err
			}

			fmt.Printf("added node %q\n", args[0])
			return nil
		},
	}

	cmd.Flags().String("addr", "", "node address (host:port)")
	cmd.Flags().String("token", "", "authentication token")
	cmd.Flags().String("project", "", "default project (optional)")

	_ = cmd.MarkFlagRequired("addr")
	_ = cmd.MarkFlagRequired("token")

	return cmd
}

func newNodesRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "remove <name>",
		Short:       "Remove a node",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			if err := config.RemoveNode(args[0]); err != nil {
				return err
			}

			fmt.Printf("removed node %q\n", args[0])
			return nil
		},
	}
}

func newNodesUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Short:       "Set the active node",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			if _, exists := cfg.Nodes[args[0]]; !exists {
				return fmt.Errorf("node %q not found", args[0])
			}

			if err := config.SetContext(args[0]); err != nil {
				return err
			}

			fmt.Printf("switched to node %q\n", args[0])
			return nil
		},
	}
}

func newNodesActiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "active",
		Short:       "Show the active node",
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			if cfg.Context == "" {
				fmt.Println("no active node")
			} else {
				fmt.Println(cfg.Context)
			}
			return nil
		},
	}
}

func newNodesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "show <name>",
		Short:       "Show node details",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			node, exists := cfg.Nodes[args[0]]
			if !exists {
				return fmt.Errorf("node %q not found", args[0])
			}

			fmt.Printf("Name:    %s\n", args[0])
			fmt.Printf("Addr:    %s\n", node.Addr)
			fmt.Printf("Token:   %s\n", maskToken(node.Token))
			if node.Project != "" {
				fmt.Printf("Project: %s\n", node.Project)
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
