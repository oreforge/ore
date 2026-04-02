package service

import (
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/engine"
	"github.com/oreforge/ore/internal/handler"
)

func newRouter(pm *engine.ProjectManager, token string, logger *slog.Logger, logLevel slog.Level, dockerClient docker.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		humaConfig := huma.DefaultConfig("ored", "1.0.0")
		humaConfig.Info.Description = "OreForge daemon REST API for managing game server networks"
		humaConfig.Servers = []*huma.Server{{URL: "/api"}}

		if token != "" {
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

		if token != "" {
			api.UseMiddleware(humaBearerAuth(api, token))
		}

		handler.RegisterRoutes(api, pm, logLevel)

		r.Group(func(r chi.Router) {
			if token != "" {
				r.Use(bearerAuth(token))
			}
			r.Use(projectResolver(pm))
			r.Get("/console", handler.Console(dockerClient))
		})
	})

	return r
}
