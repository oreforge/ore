package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

const (
	batchConcurrency = 4
	batchMaxTargets  = 100
)

func submitOperation(
	w http.ResponseWriter,
	pm *project.Manager,
	store *operation.Store,
	logger *slog.Logger,
	logLevel slog.Level,
	projectName, action, target string,
	fn func(ctx context.Context, logger *slog.Logger) error,
) {
	if _, err := pm.Resolve(projectName); err != nil {
		errs.Write(w, http.StatusNotFound, "project not found")
		return
	}

	req := operation.SubmitRequest{
		Project:  projectName,
		Action:   action,
		Target:   target,
		LogLevel: logLevel,
		Logger:   logger,
		Fn:       fn,
	}
	if target == "" {
		req.Exclusive = true
	} else {
		req.Targets = []string{target}
	}

	op, err := store.Submit(req)
	if err != nil {
		errs.Write(w, http.StatusConflict, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/operations/"+op.ID)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(dto.NewOperationResponse(op))
}

func normalizeTargets(in []string) ([]string, error) {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, errors.New("targets must not be empty")
	}
	if len(out) > batchMaxTargets {
		return nil, fmt.Errorf("too many targets (max %d)", batchMaxTargets)
	}
	return out, nil
}

func runBatch(
	ctx context.Context,
	logger *slog.Logger,
	targets []string,
	fn func(ctx context.Context, target string, logger *slog.Logger) error,
) []dto.BatchTargetResult {
	results := make([]dto.BatchTargetResult, len(targets))

	var g errgroup.Group
	g.SetLimit(batchConcurrency)

	for i, t := range targets {
		i, t := i, t
		g.Go(func() error {
			if ctx.Err() != nil {
				results[i] = dto.BatchTargetResult{Target: t, Status: "skipped", Error: "cancelled"}
				logger.Warn("batch target skipped", "target", t, "reason", "cancelled")
				return nil
			}
			tlog := logger.With("target", t)
			if err := fn(ctx, t, tlog); err != nil {
				if ctx.Err() != nil {
					results[i] = dto.BatchTargetResult{Target: t, Status: "skipped", Error: "cancelled"}
					tlog.Warn("batch target skipped", "reason", "cancelled")
					return nil
				}
				tlog.Error("batch target failed", "error", err.Error())
				results[i] = dto.BatchTargetResult{Target: t, Status: "failed", Error: err.Error()}
				return nil
			}
			tlog.Info("batch target ok")
			results[i] = dto.BatchTargetResult{Target: t, Status: "ok"}
			return nil
		})
	}
	_ = g.Wait()
	return results
}

func submitBatchOperation(
	w http.ResponseWriter,
	r *http.Request,
	store *operation.Store,
	logger *slog.Logger,
	logLevel slog.Level,
	projectName, action, target string,
	targets []string,
	perTarget func(ctx context.Context, target string, logger *slog.Logger) error,
) {
	async, _ := strconv.ParseBool(r.URL.Query().Get("async"))

	done := make(chan struct{})
	var results []dto.BatchTargetResult
	var cancelled bool
	op, err := store.Submit(operation.SubmitRequest{
		Project:  projectName,
		Action:   action,
		Target:   target,
		Targets:  targets,
		LogLevel: logLevel,
		Logger:   logger,
		Fn: func(ctx context.Context, l *slog.Logger) error {
			results = runBatch(ctx, l, targets, perTarget)
			if ctx.Err() != nil {
				cancelled = true
				close(done)
				return ctx.Err()
			}
			err := summarizeBatchResults(results)
			close(done)
			return err
		},
	})
	if err != nil {
		errs.Write(w, http.StatusConflict, err.Error())
		return
	}

	if async {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/api/operations/"+op.ID)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(dto.NewOperationResponse(op))
		return
	}

	select {
	case <-done:
	case <-r.Context().Done():
		return
	}

	status := "completed"
	resp := dto.BatchResponse{
		OperationID: op.ID,
		Total:       len(results),
		Results:     results,
	}
	for _, res := range results {
		switch res.Status {
		case "ok":
			resp.Succeeded++
		case "failed":
			resp.Failed++
		case "skipped":
			resp.Skipped++
		}
	}
	switch {
	case cancelled:
		status = "cancelled"
	case resp.Failed > 0:
		status = "failed"
	}
	resp.Status = status

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func summarizeBatchResults(results []dto.BatchTargetResult) error {
	var failed []string
	for _, r := range results {
		if r.Status == "failed" {
			failed = append(failed, r.Target)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("%d/%d targets failed: %s", len(failed), len(results), strings.Join(failed, ","))
}
