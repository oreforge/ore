package service

import (
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/handler"
)

func newRouter(cfg *config.OredConfig, logger *slog.Logger, logLevel slog.Level) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		if cfg.Auth.Token != "" {
			r.Use(bearerAuth(cfg.Auth.Token))
		}

		humaConfig := huma.DefaultConfig("ored", "1.0.0")
		humaConfig.Info.Description = "OreForge daemon REST API for managing game server networks"
		humaConfig.Servers = []*huma.Server{{URL: "/api"}}
		api := humachi.New(r, humaConfig)

		handler.RegisterRoutes(api, cfg, logLevel)

		r.Group(func(r chi.Router) {
			r.Use(projectResolver(cfg))
			r.Get("/console", handler.Console())
		})
	})

	return r
}
