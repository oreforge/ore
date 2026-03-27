package service

import (
	"log/slog"
	"os"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func Run(_ []string, info BuildInfo) int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("starting ored", "version", info.Version)
	logger.Warn("ored is not yet implemented")
	return 0
}
