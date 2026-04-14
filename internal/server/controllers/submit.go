package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

func submitOperation(
	w http.ResponseWriter,
	pm *project.Manager,
	store *operation.Store,
	logger *slog.Logger,
	logLevel slog.Level,
	name, action, target string,
	fn func(ctx context.Context, logger *slog.Logger) error,
) {
	if _, err := pm.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, "project not found")
		return
	}

	op, err := store.Submit(name, action, target, logLevel, logger, fn)
	if err != nil {
		errs.Write(w, http.StatusConflict, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/operations/"+op.ID)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(dto.NewOperationResponse(op))
}
