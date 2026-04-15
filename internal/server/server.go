package server

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"

	"github.com/oreforge/ore/internal/backup"
	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/controllers"
	mw "github.com/oreforge/ore/internal/server/middleware"
	"github.com/oreforge/ore/internal/volumes"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func New(pm *project.Manager, opStore *operation.Store, token string, logger *slog.Logger, logLevel slog.Level, dockerClient docker.Client, addr string, version string, backupsDir string) (*fuego.Server, error) {
	if strings.HasPrefix(addr, ":") {
		addr = "0.0.0.0" + addr
	}

	serverOpts := []fuego.ServerOption{
		fuego.WithAddr(addr),
		fuego.WithoutAutoGroupTags(),
		fuego.WithEngineOptions(
			fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
				DisableLocalSave: true,
				PrettyFormatJSON: true,
				MiddlewareConfig: fuego.MiddlewareConfig{
					DisableMiddlewareSection: true,
				},
				Info: &openapi3.Info{
					Title:       "ored",
					Description: "OreForge daemon REST API for managing game server networks.",
					Version:     version,
				},
			}),
		),
	}

	if token != "" {
		serverOpts = append(serverOpts,
			fuego.WithSecurity(openapi3.SecuritySchemes{
				"bearerAuth": &openapi3.SecuritySchemeRef{
					Value: openapi3.NewSecurityScheme().
						WithType("http").
						WithScheme("bearer"),
				},
			}),
		)
	}

	s := fuego.NewServer(serverOpts...)
	s.WriteTimeout = 0

	api := fuego.Group(s, "/api")
	fuego.Use(api, mw.RequestLogger(logger))

	authed := fuego.Group(api, "")
	if token != "" {
		fuego.Use(authed, mw.BearerAuth(token))
	}
	projectRes := controllers.ProjectResource{PM: pm, Store: opStore, DockerClient: dockerClient, LogLevel: logLevel, Logger: logger, Token: token}
	ops := projectRes.MountRoutes(authed)

	volSvc := volumes.New(dockerClient, logger)
	backupSvc, err := backup.NewLocalService(backupsDir, volSvc, backup.NewHelperSnapshotter(dockerClient), dockerClient, logger)
	if err != nil {
		return nil, err
	}

	controllers.ServerResource{PM: pm, Store: opStore, LogLevel: logLevel, Logger: logger}.MountRoutes(ops)
	controllers.ServiceResource{PM: pm, Store: opStore, LogLevel: logLevel, Logger: logger}.MountRoutes(ops)
	controllers.VolumeResource{PM: pm, Volumes: volSvc, Store: opStore, LogLevel: logLevel, Logger: logger}.MountRoutes(ops)
	controllers.BackupResource{PM: pm, Backups: backupSvc, Store: opStore, LogLevel: logLevel, Logger: logger}.MountRoutes(ops)

	controllers.OperationResource{Store: opStore, Logger: logger, LogLevel: logLevel}.MountRoutes(authed)

	webhookGroup := fuego.Group(api, "/webhook")
	fuego.Use(webhookGroup, mw.CORS())
	controllers.WebhookResource{PM: pm, Token: token, Logger: logger}.MountRoutes(webhookGroup)

	for _, pathItem := range s.OpenAPI.Description().Paths.Map() {
		for _, op := range pathItem.Operations() {
			filtered := make(openapi3.Parameters, 0, len(op.Parameters))
			for _, p := range op.Parameters {
				if p.Value != nil && p.Value.In == "header" && p.Value.Name == "Accept" {
					continue
				}
				filtered = append(filtered, p)
			}
			op.Parameters = filtered
		}
	}

	return s, nil
}

func Run(_ []string, info BuildInfo) int {
	cfg, err := config.LoadOred()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return 1
	}

	if err := config.EnsureToken(cfg); err != nil {
		slog.Error("failed to ensure auth token", "error", err)
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

	opStore := operation.NewStore(logger)
	pm := project.NewManager(cfg.Projects, cfg.BindMounts, logger, opStore)

	s, err := New(pm, opStore, cfg.Token, logger, level, dockerClient, cfg.Addr, info.Version, cfg.Backups)
	if err != nil {
		logger.Error("failed to initialise server", "error", err)
		return 1
	}

	pm.StartPolling()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("ored listening",
			"version", info.Version,
			"addr", cfg.Addr,
			"config", config.OredConfigFile(),
			"projects", cfg.Projects,
			"bind_mounts", cfg.BindMounts,
		)
		if listenErr := s.Run(); listenErr != nil {
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
	opStore.Stop()

	if err := s.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	pm.Shutdown(ctx)
	return 0
}
