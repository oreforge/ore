package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

func up(cfg *config.OredConfig, logLevel slog.Level) func(context.Context, *UpInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *UpInput) (*huma.StreamResponse, error) {
		specPath, err := resolveProjectInput(cfg, input.Project)
		if err != nil {
			return nil, err
		}
		return newStreamResponse(cfg, specPath, logLevel, func(eng *engine.Local) error {
			return eng.Up(ctx, input.Body.NoCache)
		}), nil
	}
}

func down(cfg *config.OredConfig, logLevel slog.Level) func(context.Context, *DownInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *DownInput) (*huma.StreamResponse, error) {
		specPath, err := resolveProjectInput(cfg, input.Project)
		if err != nil {
			return nil, err
		}
		return newStreamResponse(cfg, specPath, logLevel, func(eng *engine.Local) error {
			return eng.Down(ctx)
		}), nil
	}
}

func build(cfg *config.OredConfig, logLevel slog.Level) func(context.Context, *BuildInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *BuildInput) (*huma.StreamResponse, error) {
		specPath, err := resolveProjectInput(cfg, input.Project)
		if err != nil {
			return nil, err
		}
		return newStreamResponse(cfg, specPath, logLevel, func(eng *engine.Local) error {
			return eng.Build(ctx, input.Body.NoCache)
		}), nil
	}
}

func prune(cfg *config.OredConfig, logLevel slog.Level) func(context.Context, *PruneInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *PruneInput) (*huma.StreamResponse, error) {
		specPath, err := resolveProjectInput(cfg, input.Project)
		if err != nil {
			return nil, err
		}
		target := pruneTargetToEngine(input.Body.Target)
		return newStreamResponse(cfg, specPath, logLevel, func(eng *engine.Local) error {
			return eng.Prune(ctx, target)
		}), nil
	}
}

func clean(cfg *config.OredConfig, logLevel slog.Level) func(context.Context, *CleanInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *CleanInput) (*huma.StreamResponse, error) {
		specPath, err := resolveProjectInput(cfg, input.Project)
		if err != nil {
			return nil, err
		}
		target := cleanTargetToEngine(input.Body.Target)
		return newStreamResponse(cfg, specPath, logLevel, func(eng *engine.Local) error {
			return eng.Clean(ctx, target)
		}), nil
	}
}

func newStreamResponse(cfg *config.OredConfig, specPath string, logLevel slog.Level, fn func(*engine.Local) error) *huma.StreamResponse {
	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			ctx.SetHeader("Content-Type", "application/x-ndjson")
			ctx.SetHeader("X-Content-Type-Options", "nosniff")
			ctx.SetHeader("Cache-Control", "no-cache")

			writer := ctx.BodyWriter()
			flusher, _ := writer.(http.Flusher)

			ndjson := newNDJSONHandler(writer, flusher, logLevel)
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
			_, _ = writer.Write(line)
			if flusher != nil {
				flusher.Flush()
			}
		},
	}
}
