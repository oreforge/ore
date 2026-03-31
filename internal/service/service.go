package service

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oreforge/ore/internal/config"
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

	router := newRouter(cfg, logger, level)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var poller *Poller
	if cfg.GitPoll.Enabled {
		poller = NewPoller(cfg, logger)
		poller.Start()
	}

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

	if poller != nil {
		poller.Stop()
	}

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	shutdownProjects(ctx, cfg, logger)
	return 0
}

func shutdownProjects(ctx context.Context, cfg *config.OredConfig, logger *slog.Logger) {
	entries, err := os.ReadDir(cfg.Projects)
	if err != nil {
		logger.Error("failed to read projects directory", "error", err)
		return
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		specPath := filepath.Join(cfg.Projects, e.Name(), "ore.yaml")
		if _, statErr := os.Stat(specPath); statErr != nil {
			continue
		}

		logger.Info("stopping project", "project", e.Name())
		eng := engine.NewLocal(logger, specPath, engine.WithBindMounts(cfg.BindMounts))
		err := eng.Down(ctx)
		_ = eng.Close()
		if err != nil {
			logger.Error("failed to stop project", "project", e.Name(), "error", err)
		}
	}
}
