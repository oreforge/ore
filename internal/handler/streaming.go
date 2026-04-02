package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/engine"
)

func up(pm *engine.ProjectManager, logLevel slog.Level) func(context.Context, *UpInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *UpInput) (*huma.StreamResponse, error) {
		if _, err := pm.ResolveSpec(input.Project); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		return newStreamResponse(logLevel, func(logger *slog.Logger) error {
			return pm.Up(ctx, input.Project, engine.UpOptions{
				NoCache: input.Body.NoCache,
				Force:   input.Body.Force,
			}, logger)
		}), nil
	}
}

func down(pm *engine.ProjectManager, logLevel slog.Level) func(context.Context, *DownInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *DownInput) (*huma.StreamResponse, error) {
		if _, err := pm.ResolveSpec(input.Project); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		return newStreamResponse(logLevel, func(logger *slog.Logger) error {
			return pm.Down(ctx, input.Project, logger)
		}), nil
	}
}

func build(pm *engine.ProjectManager, logLevel slog.Level) func(context.Context, *BuildInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *BuildInput) (*huma.StreamResponse, error) {
		if _, err := pm.ResolveSpec(input.Project); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		return newStreamResponse(logLevel, func(logger *slog.Logger) error {
			return pm.Build(ctx, input.Project, input.Body.NoCache, logger)
		}), nil
	}
}

func prune(pm *engine.ProjectManager, logLevel slog.Level) func(context.Context, *PruneInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *PruneInput) (*huma.StreamResponse, error) {
		if _, err := pm.ResolveSpec(input.Project); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		target := pruneTargetToEngine(input.Body.Target)
		return newStreamResponse(logLevel, func(logger *slog.Logger) error {
			return pm.Prune(ctx, input.Project, target, logger)
		}), nil
	}
}

func clean(pm *engine.ProjectManager, logLevel slog.Level) func(context.Context, *CleanInput) (*huma.StreamResponse, error) {
	return func(ctx context.Context, input *CleanInput) (*huma.StreamResponse, error) {
		if _, err := pm.ResolveSpec(input.Project); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		target := cleanTargetToEngine(input.Body.Target)
		return newStreamResponse(logLevel, func(logger *slog.Logger) error {
			return pm.Clean(ctx, input.Project, target, logger)
		}), nil
	}
}

func newStreamResponse(logLevel slog.Level, fn func(*slog.Logger) error) *huma.StreamResponse {
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

			opErr := fn(logger)

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
