package service

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/oreforge/ore/api/orev1"
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

	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		logger.Warn("failed to parse listen address, using empty host", "addr", cfg.Addr, "error", err)
	}

	var opts []grpc.ServerOption

	if cfg.Auth.Token != "" {
		opts = append(opts,
			grpc.ChainUnaryInterceptor(unaryAuthInterceptor(cfg.Auth.Token)),
			grpc.ChainStreamInterceptor(streamAuthInterceptor(cfg.Auth.Token)),
		)
	}

	srv := grpc.NewServer(opts...)
	orev1.RegisterOreServiceServer(srv, newHandler(cfg.ProjectsDir, host, level))

	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		logger.Error("failed to listen", "addr", cfg.Addr, "error", err)
		return 1
	}

	logger.Info("ored started",
		"version", info.Version,
		"addr", cfg.Addr,
		"projects_dir", cfg.ProjectsDir,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down")

		stopped := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			logger.Warn("graceful shutdown timed out, forcing")
			srv.Stop()
		}
	}()

	if err := srv.Serve(lis); err != nil {
		logger.Error("server failed", "error", err)
		return 1
	}

	return 0
}
