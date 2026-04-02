package service

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/engine"
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

	dockerClient, err := docker.New(context.Background())
	if err != nil {
		logger.Error("failed to connect to Docker", "error", err)
		return 1
	}
	defer func() { _ = dockerClient.Close() }()

	pm := engine.NewProjectManager(cfg.Projects, cfg.BindMounts, logger)

	router := newRouter(pm, cfg.Token, logger, level, dockerClient)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	pm.StartPolling()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("ored listening",
			"version", info.Version,
			"addr", cfg.Addr,
			"config", config.OredConfigFile(),
			"projects", cfg.Projects,
		)
		if listenErr := srv.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
			errCh <- listenErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		logger.Error("server error", "error", err)
		return 1
	case <-sigCh:
	}

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pm.StopPolling()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	pm.Shutdown(ctx)
	return 0
}
