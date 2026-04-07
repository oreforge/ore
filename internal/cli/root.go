package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/client"
	"github.com/oreforge/ore/internal/config"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

var (
	cfg    *config.OreConfig
	logger *slog.Logger
)

func Run(args []string, info BuildInfo) int {
	root := &cobra.Command{
		Use:   "ore",
		Short: "Infrastructure-as-code for game server networks",
		Long:  "Infrastructure-as-code for game server networks",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			cfg, err = config.LoadOre(cmd.Flags())
			if err != nil {
				return err
			}

			level := slog.LevelInfo
			if cfg.Verbose {
				level = slog.LevelDebug
			}
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
			slog.SetDefault(logger)

			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringP("file", "f", "ore.yaml", "path to ore.yaml spec file")
	root.PersistentFlags().BoolP("verbose", "v", false, "enable debug logging")

	root.AddCommand(
		newUpCmd(),
		newDownCmd(),
		newStatusCmd(),
		newBuildCmd(),
		newCleanCmd(),
		newConsoleCmd(),
		newProjectsCmd(),
		newNodesCmd(),
		newVersionCmd(info),
	)

	var err error
	cfg, err = config.LoadOre(nil)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}

	root.Long = buildLongDescription(cfg)
	root.SetArgs(args[1:])
	err = root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func resolveMode(cmd *cobra.Command) (local bool, specPath string, remote *client.Client, err error) {
	specFile, _ := cmd.Flags().GetString("file")
	if _, statErr := os.Stat(specFile); statErr == nil {
		logger.Debug("using local spec", "file", specFile)
		return true, specFile, nil, nil
	}

	addr, token, project, ok := config.ResolveRemote(cfg)
	if !ok {
		return false, "", nil, fmt.Errorf("no %s found and no remote node configured", specFile)
	}

	if project == "" {
		return false, "", nil, fmt.Errorf("no active project set (use 'ore projects use <name>' to select one)")
	}

	c, err := client.New(addr, token, project)
	if err != nil {
		return false, "", nil, fmt.Errorf("connecting to ored: %w", err)
	}

	logger.Debug("using remote node", "addr", addr, "project", project)
	return false, "", c, nil
}

func connectNode() (*client.Client, error) {
	addr, token, project, ok := config.ResolveRemote(cfg)
	if !ok {
		return nil, fmt.Errorf("no remote node configured (use 'ore nodes add' to set one up)")
	}

	c, err := client.New(addr, token, project)
	if err != nil {
		return nil, fmt.Errorf("connecting to ored: %w", err)
	}

	return c, nil
}

func buildLongDescription(cfg *config.OreConfig) string {
	desc := "Infrastructure-as-code for game server networks\n\n"
	desc += "Config:  " + config.OreConfigFile() + "\n"

	if cfg.Context != "" {
		desc += "Node:    " + cfg.Context + "\n"
		if node, ok := cfg.Nodes[cfg.Context]; ok && node.Project != "" {
			desc += "Project: " + node.Project
		}
	}

	return desc
}
