package handler

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

func getStatus(cfg *config.OredConfig) func(context.Context, *ProjectInput) (*StatusOutput, error) {
	return func(ctx context.Context, input *ProjectInput) (*StatusOutput, error) {
		specPath, err := resolveProjectInput(cfg, input.Project)
		if err != nil {
			return nil, err
		}

		eng := engine.NewLocal(slog.Default(), specPath, engine.WithBindMounts(cfg.BindMounts))
		status, engErr := eng.Status(ctx)
		if engErr != nil {
			return nil, huma.Error500InternalServerError(engErr.Error())
		}

		return &StatusOutput{Body: *status}, nil
	}
}
