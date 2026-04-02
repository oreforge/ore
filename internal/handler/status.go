package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/engine"
)

func getStatus(pm *engine.ProjectManager) func(context.Context, *ProjectInput) (*StatusOutput, error) {
	return func(ctx context.Context, input *ProjectInput) (*StatusOutput, error) {
		status, err := pm.Status(ctx, input.Project)
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &StatusOutput{Body: *status}, nil
	}
}
