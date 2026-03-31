package service

import (
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/handler"
)

func newRouter(cfg *config.OredConfig, logger *slog.Logger, logLevel slog.Level, dockerClient docker.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		humaConfig := huma.DefaultConfig("ored", "1.0.0")
		humaConfig.Info.Description = "OreForge daemon REST API for managing game server networks"
		humaConfig.Servers = []*huma.Server{{URL: "/api"}}

		if cfg.Token != "" {
			humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
				"bearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "token",
					Description:  "Static API token configured on the ored server",
				},
			}
			humaConfig.Security = []map[string][]string{
				{"bearerAuth": {}},
			}
		}

		api := humachi.New(r, humaConfig)

		if cfg.Token != "" {
			api.UseMiddleware(humaBearerAuth(api, cfg.Token))
		}

		handler.RegisterRoutes(api, cfg, logLevel)

		r.Group(func(r chi.Router) {
			if cfg.Token != "" {
				r.Use(bearerAuth(cfg.Token))
			}
			r.Use(projectResolver(cfg))
			r.Get("/console", handler.Console(dockerClient))
		})
	})

	return r
}
