package controllers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
	"github.com/oreforge/ore/internal/spec"
	"github.com/oreforge/ore/internal/webhook"
)

func parseBoolQuery(r *http.Request, key string, fallback bool) bool {
	v := r.URL.Query().Get(key)
	switch v {
	case "true", "1":
		return true
	case "false", "0":
		return false
	default:
		return fallback
	}
}

type WebhookResource struct {
	PM     *project.Manager
	Token  string
	Logger *slog.Logger
}

func (rs WebhookResource) MountRoutes(s *fuego.Server) {
	fuego.PostStd(s, "/{name}", rs.handle,
		option.Summary("GitOps webhook"),
		option.Description("Triggers a deploy for a project. Authenticated via secret query parameter derived from HMAC-SHA256(token, project_name)."),
		option.Tags("Webhook"),
		option.OperationID("webhookDeploy"),
		option.Path("name", "Project name"),
		option.Query("secret", "HMAC-derived webhook secret"),
		option.Query("force", "Force restart all servers even if unchanged (overrides spec default)"),
		option.Query("no_cache", "Skip local binary cache and re-download everything (overrides spec default)"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusUnauthorized, "Invalid or missing secret", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Deploy already in progress", fuego.Response{Type: fuego.HTTPError{}}),
	)
}

func (rs WebhookResource) handle(w http.ResponseWriter, r *http.Request) {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()

	name := r.PathValue("name")
	secret := r.URL.Query().Get("secret")

	if secret == "" {
		errs.Write(w, http.StatusUnauthorized, "missing secret parameter")
		return
	}

	specPath, err := rs.PM.Resolve(name)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	s, err := spec.Load(specPath)
	if err != nil {
		rs.Logger.Error("failed to load spec", "project", name, "error", err)
		errs.Write(w, http.StatusInternalServerError, "failed to load project spec")
		return
	}

	if s.GitOps == nil || !s.GitOps.Webhook.Enabled {
		errs.Write(w, http.StatusNotFound, "webhook not enabled for this project")
		return
	}

	if !webhook.ValidateSecret(rs.Token, name, secret) {
		errs.Write(w, http.StatusUnauthorized, "invalid secret")
		return
	}

	opts := project.UpOptions{
		Force:   parseBoolQuery(r, "force", s.GitOps.Webhook.Force),
		NoCache: parseBoolQuery(r, "no_cache", s.GitOps.Webhook.NoCache),
	}

	if !rs.PM.TriggerDeploy(name, opts) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(dto.WebhookResponse{
			Status:  "skipped",
			Project: name,
		})
		return
	}

	rs.Logger.Info("webhook triggered deploy", "project", name)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(dto.WebhookResponse{
		Status:  "accepted",
		Project: name,
	})
}
