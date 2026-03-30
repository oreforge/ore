package service

import (
	"log/slog"

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

		r.Get("/projects", handler.ListProjects(cfg))

		r.Group(func(r chi.Router) {
			r.Use(projectResolver(cfg))

			r.Get("/status", handler.Status(cfg))
			r.Post("/up", handler.Up(cfg, logLevel))
			r.Post("/down", handler.Down(cfg, logLevel))
			r.Post("/build", handler.Build(cfg, logLevel))
			r.Post("/prune", handler.Prune(cfg, logLevel))
			r.Post("/clean", handler.Clean(cfg, logLevel))
			r.Get("/console", handler.Console())
		})
	})

	return r
}
