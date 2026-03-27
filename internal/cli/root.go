package cli

import (
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
	configFile string
	eng        engine.Engine
)

func Run(args []string, info BuildInfo) int {
	root := &cobra.Command{
		Use:   "ore",
		Short: "Infrastructure-as-code for game server networks",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadOre(cmd.Flags())
			if err != nil {
				return err
			}

			configFile = cfg.File

			level := slog.LevelInfo
			if cfg.Verbose {
				level = slog.LevelDebug
			}
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
			slog.SetDefault(logger)

			eng = engine.NewLocal(logger)

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
		newVersionCmd(info),
	)

	root.SetArgs(args[1:])
	if err := root.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		return 1
	}
	return 0
}
