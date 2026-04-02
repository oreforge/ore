package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/engine"
	"github.com/oreforge/ore/internal/orchestrator"
)

type ProjectInput struct {
	Project string `header:"X-Ore-Project" required:"true" doc:"Active project name"`
}

type ProjectsOutput struct {
	Body struct {
		Projects []string `json:"projects" doc:"List of project names"`
	}
}

type StatusOutput struct {
	Body orchestrator.NetworkStatus
}

type NoCacheBody struct {
	NoCache bool `json:"no_cache,omitempty" doc:"Skip local binary cache"`
}

type UpBody struct {
	NoCache bool `json:"no_cache,omitempty" doc:"Skip local binary cache"`
	Force   bool `json:"force,omitempty" doc:"Force restart all containers even if unchanged"`
}

type UpInput struct {
	ProjectInput
	Body UpBody
}

type DownInput struct {
	ProjectInput
}

type BuildInput struct {
	ProjectInput
	Body NoCacheBody
}

type PruneInput struct {
	ProjectInput
	Body struct {
		Target string `json:"target,omitempty" enum:"all,containers,images,volumes" default:"all" doc:"Resource type to prune"`
	}
}

type CleanInput struct {
	ProjectInput
	Body struct {
		Target string `json:"target,omitempty" enum:"all,cache,builds" default:"all" doc:"Artifact type to clean"`
	}
}

func RegisterRoutes(api huma.API, pm *engine.ProjectManager, logLevel slog.Level) {
	ndjsonDesc := "Streams progress as NDJSON (application/x-ndjson). Final line contains {\"done\":true} with an optional \"error\" field."

	huma.Register(api, huma.Operation{
		OperationID: "list-projects",
		Summary:     "List available projects",
		Method:      http.MethodGet,
		Path:        "/projects",
		Tags:        []string{"Projects"},
	}, listProjects(pm))

	huma.Register(api, huma.Operation{
		OperationID: "add-project",
		Summary:     "Clone a project from a git repository",
		Method:      http.MethodPost,
		Path:        "/projects",
		Tags:        []string{"Projects"},
	}, addProject(pm))

	huma.Register(api, huma.Operation{
		OperationID:   "remove-project",
		Summary:       "Stop containers and remove a project",
		Method:        http.MethodDelete,
		Path:          "/projects/{name}",
		Tags:          []string{"Projects"},
		DefaultStatus: http.StatusNoContent,
	}, removeProject(pm))

	huma.Register(api, huma.Operation{
		OperationID: "update-project",
		Summary:     "Pull latest changes and redeploy",
		Method:      http.MethodPatch,
		Path:        "/projects/{name}",
		Tags:        []string{"Projects"},
	}, updateProject(pm))

	huma.Register(api, huma.Operation{
		OperationID: "get-status",
		Summary:     "Get network and container status",
		Method:      http.MethodGet,
		Path:        "/status",
		Tags:        []string{"Operations"},
	}, getStatus(pm))

	huma.Register(api, huma.Operation{
		OperationID: "up",
		Summary:     "Build images and start the network",
		Description: ndjsonDesc,
		Method:      http.MethodPost,
		Path:        "/up",
		Tags:        []string{"Operations"},
	}, up(pm, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "down",
		Summary:     "Stop all containers and remove the network",
		Description: ndjsonDesc,
		Method:      http.MethodPost,
		Path:        "/down",
		Tags:        []string{"Operations"},
	}, down(pm, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "build",
		Summary:     "Build Docker images for all servers",
		Description: ndjsonDesc,
		Method:      http.MethodPost,
		Path:        "/build",
		Tags:        []string{"Operations"},
	}, build(pm, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "prune",
		Summary:     "Remove unused resources",
		Description: ndjsonDesc,
		Method:      http.MethodPost,
		Path:        "/prune",
		Tags:        []string{"Operations"},
	}, prune(pm, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "clean",
		Summary:     "Remove cache and build artifacts",
		Description: ndjsonDesc,
		Method:      http.MethodPost,
		Path:        "/clean",
		Tags:        []string{"Operations"},
	}, clean(pm, logLevel))
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"status":%d,"detail":%q}`, status, msg)
}

func listProjects(pm *engine.ProjectManager) func(context.Context, *struct{}) (*ProjectsOutput, error) {
	return func(_ context.Context, _ *struct{}) (*ProjectsOutput, error) {
		names, err := pm.ListProjects()
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		out := &ProjectsOutput{}
		out.Body.Projects = names
		return out, nil
	}
}

func pruneTargetToEngine(s string) engine.PruneTarget {
	switch s {
	case "containers":
		return engine.PruneContainers
	case "images":
		return engine.PruneImages
	case "volumes":
		return engine.PruneVolumes
	default:
		return engine.PruneAll
	}
}

func cleanTargetToEngine(s string) engine.CleanTarget {
	switch s {
	case "cache":
		return engine.CleanCache
	case "builds":
		return engine.CleanBuilds
	default:
		return engine.CleanAll
	}
}
