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
		newProjectsUseCmd(),
		newProjectsUpdateCmd(),
		newProjectsRemoveCmd(),
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
		Use:     "list",
		Short:   "List remote projects",
		Example: "ore projects list",
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
		Use:     "update <name>",
		Short:   "Pull latest changes and redeploy",
		Example: "ore projects update my-network",
		Args:    cobra.ExactArgs(1),
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
		Use:     "use <name>",
		Short:   "Set the active project",
		Example: "ore projects use my-network",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := requireRemote()
			if err != nil {
				return err
			}

			projects, err := r.ListProjects(cmd.Context())
			if err != nil {
				return err
			}

			found := false
			for _, p := range projects {
				if p == args[0] {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("project %q not found on remote node", args[0])
			}

			if err := config.SetProject(args[0]); err != nil {
				return err
			}
			fmt.Printf("switched to project %q\n", args[0])
			return nil
		},
	}
}

func newProjectsWebhookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "webhook <name>",
		Short:   "Show the webhook URL for a project",
		Example: "ore projects webhook my-network",
		Args:    cobra.ExactArgs(1),
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

			webhookURL := info.URL

			force, _ := cmd.Flags().GetBool("force")
			noCache, _ := cmd.Flags().GetBool("no-cache")

			if force {
				webhookURL += "&force=true"
			}
			if noCache {
				webhookURL += "&no_cache=true"
			}

			addr, _, _, _ := config.ResolveRemote(cfg)
			fmt.Printf("%s%s\n", addr, webhookURL)
			return nil
		},
	}

	cmd.Flags().Bool("no-cache", false, "include no_cache=true parameter in the URL")
	cmd.Flags().Bool("force", false, "include force=true parameter in the URL")

	return cmd
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
