package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

var (
	eng engine.Engine
	cfg *config.OreConfig
)

func Run(args []string, info BuildInfo) int {
	root := &cobra.Command{
		Use:   "ore",
		Short: "Infrastructure-as-code for game server networks",
		Long:  "Infrastructure-as-code for game server networks\n\nConfig:\n" + config.OreConfigFile(),
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
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
			slog.SetDefault(logger)

			if cmd.Annotations["skip-engine"] == "true" {
				return nil
			}

			specFile, _ := cmd.Flags().GetString("file")
			if _, statErr := os.Stat(specFile); statErr == nil {
				eng = engine.NewLocal(logger, specFile)
			} else if addr, token, project, ok := config.ResolveRemote(cfg); ok {
				var remoteErr error
				eng, remoteErr = engine.NewRemote(addr, token, project)
				if remoteErr != nil {
					return fmt.Errorf("connecting to ored: %w", remoteErr)
				}
			} else {
				return fmt.Errorf("no %s found and no remote server configured", specFile)
			}

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
		newPruneCmd(),
		newCleanCmd(),
		newConsoleCmd(),
		newProjectsCmd(),
		newServersCmd(),
		newVersionCmd(info),
	)

	var err error
	cfg, err = config.LoadOre(nil)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}

	root.SetArgs(args[1:])
	err = root.Execute()
	if eng != nil {
		_ = eng.Close()
	}
	if err != nil {
		slog.Error("command failed", "error", err)
		return 1
	}
	return 0
}
