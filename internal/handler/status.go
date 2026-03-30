package handler

import (
	"log/slog"
	"net/http"

	"github.com/oreforge/ore/internal/config"
	"github.com/oreforge/ore/internal/engine"
)

func Status(cfg *config.OredConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		specPath := SpecPathFromCtx(r.Context())

		eng := engine.NewLocal(slog.Default(), specPath, engine.WithBindMounts(cfg.BindMounts))
		status, err := eng.Status(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, status)
	}
}
