package service

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/oreforge/ore/internal/config"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func Run(_ []string, info BuildInfo) int {
	cfg, err := config.LoadOred()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("ored: not implemented",
		"version", info.Version,
		"addr", cfg.Addr,
		"projects_dir", cfg.ProjectsDir,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down")
	return 0
}
