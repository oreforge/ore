package service

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/handler"
)

func humaBearerAuth(api huma.API, token string) func(ctx huma.Context, next func(huma.Context)) {
	expected := []byte(token)
	return func(ctx huma.Context, next func(huma.Context)) {
		auth := ctx.Header("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		got := []byte(strings.TrimPrefix(auth, "Bearer "))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid token")
			return
		}
		next(ctx)
	}
}

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

			specPath, err := handler.ResolveProject(cfg, project)
			if err != nil {
				handler.WriteError(w, http.StatusNotFound, err.Error())
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
