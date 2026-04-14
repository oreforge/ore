package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	op "github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

type OperationResource struct {
	Store    *op.Store
	Logger   *slog.Logger
	LogLevel slog.Level
}

func (rs OperationResource) MountRoutes(s *fuego.Server) {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	ops := fuego.Group(s, "/operations")

	fuego.Get(ops, "", rs.list,
		option.Summary("List operations"),
		option.Description("Returns all operations, optionally filtered by project or status."),
		option.Tags("Operations"),
		option.OperationID("listOperations"),
		option.Query("project", "Filter by project name"),
		option.Query("status", "Filter by status (pending, running, completed, failed, cancelled)"),
		bearer,
	)
	fuego.Get(ops, "/{id}", rs.get,
		option.Summary("Get operation"),
		option.Description("Returns the status of a single operation."),
		option.Tags("Operations"),
		option.OperationID("getOperation"),
		option.Path("id", "Operation ID"),
		option.AddResponse(http.StatusNotFound, "Operation not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.GetStd(ops, "/{id}/logs", rs.logs,
		option.Summary("Stream operation logs"),
		option.OverrideDescription("Streams operation logs as NDJSON (application/x-ndjson). Use cursor query parameter to resume from a position. Final line contains {\"done\":true} with an optional \"error\" field."),
		option.Tags("Operations"),
		option.OperationID("streamOperationLogs"),
		option.Path("id", "Operation ID"),
		option.Query("cursor", "Log cursor position (default 0)"),
		option.AddResponse(http.StatusOK, "NDJSON log stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Operation not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(ops, "/{id}/cancel", rs.cancel,
		option.Summary("Cancel operation"),
		option.Description("Cancels a running operation."),
		option.Tags("Operations"),
		option.OperationID("cancelOperation"),
		option.Path("id", "Operation ID"),
		option.AddResponse(http.StatusNotFound, "Operation not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already finished", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs OperationResource) list(c fuego.ContextNoBody) (dto.OperationListResponse, error) {
	project := c.QueryParam("project")
	statusFilter := c.QueryParam("status")

	all := rs.Store.List(project)
	filtered := make([]dto.OperationResponse, 0, len(all))
	for _, o := range all {
		snap := o.Snapshot()
		if statusFilter != "" && string(snap.Status) != statusFilter {
			continue
		}
		filtered = append(filtered, dto.NewOperationResponse(o))
	}
	return dto.OperationListResponse{Operations: filtered}, nil
}

func (rs OperationResource) get(c fuego.ContextNoBody) (dto.OperationResponse, error) {
	id := c.PathParam("id")
	o, ok := rs.Store.Get(id)
	if !ok {
		return dto.OperationResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "operation not found"}
	}
	return dto.NewOperationResponse(o), nil
}

func (rs OperationResource) logs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o, ok := rs.Store.Get(id)
	if !ok {
		errs.Write(w, http.StatusNotFound, "operation not found")
		return
	}

	cursor := 0
	if v := r.URL.Query().Get("cursor"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cursor = n
		}
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := w.(http.Flusher)
	buf := o.LogBuffer()
	ctx := r.Context()

	for {
		lines, next, done := buf.Wait(ctx, cursor)
		if ctx.Err() != nil {
			return
		}
		cursor = next
		for _, line := range lines {
			if _, err := w.Write(line); err != nil {
				return
			}
		}
		if flusher != nil && len(lines) > 0 {
			flusher.Flush()
		}
		if done {
			result := map[string]any{"done": true}
			if errMsg := o.ErrorMsg(); errMsg != "" {
				result["error"] = errMsg
			}
			line, _ := json.Marshal(result)
			line = append(line, '\n')
			_, _ = w.Write(line)
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
	}
}

func (rs OperationResource) cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := rs.Store.Cancel(id)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, op.ErrNotFound):
		errs.Write(w, http.StatusNotFound, "operation not found")
	case errors.Is(err, op.ErrFinished):
		errs.Write(w, http.StatusConflict, "operation already finished")
	default:
		rs.Logger.Error("cancel failed", "id", id, "error", err)
		errs.Write(w, http.StatusInternalServerError, "cancel failed")
	}
}
