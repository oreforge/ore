package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

func newProjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Manage remote projects",
	}

	cmd.AddCommand(
		newProjectsListCmd(),
		newProjectsUseCmd(),
		newProjectsActiveCmd(),
	)

	return cmd
}

func remoteEngine() (*engine.Remote, error) {
	r, ok := eng.(*engine.Remote)
	if !ok {
		return nil, fmt.Errorf("project management is only available in remote mode")
	}
	return r, nil
}

func newProjectsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := remoteEngine()
			if err != nil {
				return err
			}

			cfg, err := config.LoadOre(nil)
			if err != nil {
				return err
			}
			active := cfg.Remote.Project

			projects, err := r.ListProjects(cmd.Context())
			if err != nil {
				return err
			}

			for _, p := range projects {
				if p == active {
					fmt.Printf("* %s\n", p)
				} else {
					fmt.Printf("  %s\n", p)
				}
			}
			return nil
		},
	}
}

func newProjectsUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Short:       "Set the active project",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, args []string) error {
			if err := config.SaveProject(args[0]); err != nil {
				return fmt.Errorf("saving project: %w", err)
			}
			fmt.Printf("switched to project %q\n", args[0])
			return nil
		},
	}
}

func newProjectsActiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "active",
		Short:       "Show the active project",
		Annotations: map[string]string{"skip-engine": "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadOre(nil)
			if err != nil {
				return err
			}

			if cfg.Remote.Project == "" {
				fmt.Println("no active project")
			} else {
				fmt.Println(cfg.Remote.Project)
			}
			return nil
		},
	}
}
