package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/config"
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

type UpInput struct {
	ProjectInput
	Body NoCacheBody
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

func RegisterRoutes(api huma.API, cfg *config.OredConfig, logLevel slog.Level) {
	huma.Register(api, huma.Operation{
		OperationID: "list-projects",
		Summary:     "List available projects",
		Method:      http.MethodGet,
		Path:        "/projects",
		Tags:        []string{"Projects"},
	}, listProjects(cfg))

	huma.Register(api, huma.Operation{
		OperationID: "add-project",
		Summary:     "Clone a project from a git repository",
		Method:      http.MethodPost,
		Path:        "/projects",
		Tags:        []string{"Projects"},
	}, addProject(cfg))

	huma.Register(api, huma.Operation{
		OperationID: "remove-project",
		Summary:     "Stop containers and remove a project",
		Method:      http.MethodDelete,
		Path:        "/projects/{name}",
		Tags:        []string{"Projects"},
	}, removeProject(cfg))

	huma.Register(api, huma.Operation{
		OperationID: "update-project",
		Summary:     "Pull latest changes from git",
		Method:      http.MethodPatch,
		Path:        "/projects/{name}",
		Tags:        []string{"Projects"},
	}, updateProject(cfg))

	huma.Register(api, huma.Operation{
		OperationID: "get-status",
		Summary:     "Get network and container status",
		Method:      http.MethodGet,
		Path:        "/status",
		Tags:        []string{"Operations"},
	}, getStatus(cfg))

	huma.Register(api, huma.Operation{
		OperationID: "up",
		Summary:     "Build images and start the network",
		Method:      http.MethodPost,
		Path:        "/up",
		Tags:        []string{"Operations"},
	}, up(cfg, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "down",
		Summary:     "Stop all containers and remove the network",
		Method:      http.MethodPost,
		Path:        "/down",
		Tags:        []string{"Operations"},
	}, down(cfg, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "build",
		Summary:     "Build Docker images for all servers",
		Method:      http.MethodPost,
		Path:        "/build",
		Tags:        []string{"Operations"},
	}, build(cfg, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "prune",
		Summary:     "Remove unused resources",
		Method:      http.MethodPost,
		Path:        "/prune",
		Tags:        []string{"Operations"},
	}, prune(cfg, logLevel))

	huma.Register(api, huma.Operation{
		OperationID: "clean",
		Summary:     "Remove cache and build artifacts",
		Method:      http.MethodPost,
		Path:        "/clean",
		Tags:        []string{"Operations"},
	}, clean(cfg, logLevel))
}

func ResolveProject(cfg *config.OredConfig, project string) (string, error) {
	if filepath.Base(project) != project {
		return "", fmt.Errorf("invalid project name")
	}
	specPath := filepath.Join(cfg.Projects, project, "ore.yaml")
	if _, err := os.Stat(specPath); err != nil {
		return "", fmt.Errorf("project %q not found", project)
	}
	return specPath, nil
}

func resolveProjectInput(cfg *config.OredConfig, project string) (string, error) {
	specPath, err := ResolveProject(cfg, project)
	if err != nil {
		return "", huma.Error404NotFound(err.Error())
	}
	return specPath, nil
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"status":%d,"detail":%q}`, status, msg)
}

func listProjects(cfg *config.OredConfig) func(context.Context, *struct{}) (*ProjectsOutput, error) {
	return func(_ context.Context, _ *struct{}) (*ProjectsOutput, error) {
		entries, err := os.ReadDir(cfg.Projects)
		if err != nil {
			return nil, huma.Error500InternalServerError("reading projects directory: " + err.Error())
		}

		out := &ProjectsOutput{}
		for _, e := range entries {
			if !e.IsDir() || e.Name()[0] == '.' {
				continue
			}
			specFile := filepath.Join(cfg.Projects, e.Name(), "ore.yaml")
			if _, statErr := os.Stat(specFile); statErr == nil {
				out.Body.Projects = append(out.Body.Projects, e.Name())
			}
		}
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
