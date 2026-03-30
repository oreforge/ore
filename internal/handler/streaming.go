package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

func Up(cfg *config.OredConfig, logLevel slog.Level) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			NoCache bool `json:"no_cache"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		streamOperation(w, r, cfg, logLevel, func(eng *engine.Local) error {
			return eng.Up(r.Context(), req.NoCache)
		})
	}
}

func Down(cfg *config.OredConfig, logLevel slog.Level) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		streamOperation(w, r, cfg, logLevel, func(eng *engine.Local) error {
			return eng.Down(r.Context())
		})
	}
}

func Build(cfg *config.OredConfig, logLevel slog.Level) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			NoCache bool `json:"no_cache"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		streamOperation(w, r, cfg, logLevel, func(eng *engine.Local) error {
			return eng.Build(r.Context(), req.NoCache)
		})
	}
}

func Prune(cfg *config.OredConfig, logLevel slog.Level) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target string `json:"target"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		target, err := parsePruneTarget(req.Target)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		streamOperation(w, r, cfg, logLevel, func(eng *engine.Local) error {
			return eng.Prune(r.Context(), target)
		})
	}
}

func Clean(cfg *config.OredConfig, logLevel slog.Level) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target string `json:"target"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		target, err := parseCleanTarget(req.Target)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		streamOperation(w, r, cfg, logLevel, func(eng *engine.Local) error {
			return eng.Clean(r.Context(), target)
		})
	}
}

func streamOperation(w http.ResponseWriter, r *http.Request, cfg *config.OredConfig, logLevel slog.Level, fn func(*engine.Local) error) {
	specPath := SpecPathFromCtx(r.Context())

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	ndjson := newNDJSONHandler(w, flusher, logLevel)
	serverHandler := slog.Default().Handler()
	logger := slog.New(newTeeHandler(ndjson, serverHandler))

	eng := engine.NewLocal(logger, specPath, engine.WithBindMounts(cfg.BindMounts))

	opErr := fn(eng)

	result := map[string]any{"done": true}
	if opErr != nil {
		result["error"] = opErr.Error()
	}
	line, _ := json.Marshal(result)
	line = append(line, '\n')
	_, _ = w.Write(line)
	if flusher != nil {
		flusher.Flush()
	}
}

func parsePruneTarget(s string) (engine.PruneTarget, error) {
	switch s {
	case "all", "":
		return engine.PruneAll, nil
	case "containers":
		return engine.PruneContainers, nil
	case "images":
		return engine.PruneImages, nil
	case "volumes":
		return engine.PruneVolumes, nil
	default:
		return 0, fmt.Errorf("unknown prune target %q (use: all, containers, images, volumes)", s)
	}
}

func parseCleanTarget(s string) (engine.CleanTarget, error) {
	switch s {
	case "all", "":
		return engine.CleanAll, nil
	case "cache":
		return engine.CleanCache, nil
	case "builds":
		return engine.CleanBuilds, nil
	default:
		return 0, fmt.Errorf("unknown clean target %q (use: all, cache, builds)", s)
	}
}
