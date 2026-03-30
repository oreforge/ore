package service

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/handler"
)

func bearerAuth(token string) func(http.Handler) http.Handler {
	expected := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				handler.WriteError(w, http.StatusUnauthorized, "missing or invalid authorization header")
				return
			}
			got := []byte(strings.TrimPrefix(auth, "Bearer "))
			if subtle.ConstantTimeCompare(got, expected) != 1 {
				handler.WriteError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func projectResolver(cfg *config.OredConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			project := r.Header.Get("X-Ore-Project")
			if project == "" {
				handler.WriteError(w, http.StatusBadRequest, "missing X-Ore-Project header (use 'ore projects use <name>')")
				return
			}

			if filepath.Base(project) != project {
				handler.WriteError(w, http.StatusBadRequest, "invalid project name")
				return
			}

			specPath := filepath.Join(cfg.Projects, project, "ore.yaml")
			if _, err := os.Stat(specPath); err != nil {
				handler.WriteError(w, http.StatusNotFound, "project "+project+" not found")
				return
			}

			ctx := handler.WithSpecPath(r.Context(), specPath)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			reqID := middleware.GetReqID(r.Context())
			project := r.Header.Get("X-Ore-Project")

			logger.Info("request started",
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"project", project,
				"remote_addr", r.RemoteAddr,
				"request_id", reqID,
			)

			next.ServeHTTP(ww, r)

			logger.Info("request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"project", project,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start).String(),
				"request_id", reqID,
			)
		})
	}
}
