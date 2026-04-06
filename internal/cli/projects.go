package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/client"
	"github.com/oreforge/ore/internal/config"
)

func newProjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Manage remote projects",
	}

	cmd.AddCommand(
		newProjectsListCmd(),
		newProjectsAddCmd(),
		newProjectsRemoveCmd(),
		newProjectsUpdateCmd(),
		newProjectsUseCmd(),
		newProjectsActiveCmd(),
		newProjectsWebhookCmd(),
	)

	return cmd
}

func requireRemote() (*client.Client, error) {
	if remoteClient == nil {
		return nil, fmt.Errorf("project management is only available in remote mode")
	}
	return remoteClient, nil
}

func newProjectsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := requireRemote()
			if err != nil {
				return err
			}

			_, _, active, _ := config.ResolveRemote(cfg)

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

func newProjectsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add <url>",
		Short:   "Clone a project from a git repository",
		Example: "ore projects add https://github.com/user/repo.git",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := requireRemote()
			if err != nil {
				return err
			}

			name, _ := cmd.Flags().GetString("name")
			projectName, err := r.AddProject(cmd.Context(), args[0], name)
			if err != nil {
				return err
			}

			fmt.Printf("added project %q\n", projectName)
			return nil
		},
	}

	cmd.Flags().String("name", "", "custom project name (derived from URL if empty)")

	return cmd
}

func newProjectsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Stop servers and remove a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := requireRemote()
			if err != nil {
				return err
			}

			if err := r.RemoveProject(cmd.Context(), args[0]); err != nil {
				return err
			}

			fmt.Printf("removed project %q\n", args[0])
			return nil
		},
	}
}

func newProjectsUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <name>",
		Short: "Pull latest changes from git",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := requireRemote()
			if err != nil {
				return err
			}

			if err := r.UpdateProject(cmd.Context(), args[0]); err != nil {
				return err
			}

			fmt.Printf("updated project %q\n", args[0])
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
			if err := config.SetProject(args[0]); err != nil {
				return err
			}
			fmt.Printf("switched to project %q\n", args[0])
			return nil
		},
	}
}

func newProjectsWebhookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "webhook <name>",
		Short: "Show webhook URL for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := requireRemote()
			if err != nil {
				return err
			}

			info, err := r.WebhookInfo(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if !info.Enabled {
				return fmt.Errorf("webhook is not enabled for project %q", args[0])
			}

			addr, _, _, _ := config.ResolveRemote(cfg)
			fmt.Printf("http://%s%s\n", addr, info.URL)
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
			_, _, project, _ := config.ResolveRemote(cfg)
			if project == "" {
				fmt.Println("no active project")
			} else {
				fmt.Println(project)
			}
			return nil
		},
	}
}
