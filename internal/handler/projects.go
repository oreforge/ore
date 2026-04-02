package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/oreforge/ore/internal/engine"
)

type AddProjectInput struct {
	Body struct {
		URL  string `json:"url" required:"true" doc:"Git repository URL"`
		Name string `json:"name,omitempty" doc:"Custom project name (derived from URL if empty)"`
	}
}

type AddProjectOutput struct {
	Body struct {
		Name string `json:"name" doc:"Project name"`
	}
}

type ProjectPathInput struct {
	Name string `path:"name" doc:"Project name"`
}

type UpdateProjectOutput struct {
	Body struct {
		Name   string `json:"name" doc:"Project name"`
		Status string `json:"status" doc:"Update result"`
	}
}

func addProject(pm *engine.ProjectManager) func(context.Context, *AddProjectInput) (*AddProjectOutput, error) {
	return func(ctx context.Context, input *AddProjectInput) (*AddProjectOutput, error) {
		name := input.Body.Name
		if name == "" {
			var err error
			name, err = nameFromURL(input.Body.URL)
			if err != nil {
				return nil, huma.Error400BadRequest("invalid repository URL: " + err.Error())
			}
		}

		if filepath.Base(name) != name {
			return nil, huma.Error400BadRequest("invalid project name")
		}

		projectDir := filepath.Join(pm.ProjectsDir(), name)
		if _, err := os.Stat(projectDir); err == nil {
			return nil, huma.Error409Conflict("project " + name + " already exists")
		}

		cmd := exec.CommandContext(ctx, "git", "clone", input.Body.URL, projectDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			_ = os.RemoveAll(projectDir)
			return nil, huma.Error422UnprocessableEntity("git clone failed: " + strings.TrimSpace(string(output)))
		}

		specPath := filepath.Join(projectDir, "ore.yaml")
		if _, err := os.Stat(specPath); err != nil {
			_ = os.RemoveAll(projectDir)
			return nil, huma.Error422UnprocessableEntity("repository does not contain an ore.yaml")
		}

		return &AddProjectOutput{Body: struct {
			Name string `json:"name" doc:"Project name"`
		}{Name: name}}, nil
	}
}

func removeProject(pm *engine.ProjectManager) func(context.Context, *ProjectPathInput) (*struct{}, error) {
	return func(ctx context.Context, input *ProjectPathInput) (*struct{}, error) {
		if _, err := pm.ResolveSpec(input.Name); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}

		_ = pm.Down(ctx, input.Name, slog.Default())

		projectDir := filepath.Join(pm.ProjectsDir(), input.Name)
		if err := os.RemoveAll(projectDir); err != nil {
			return nil, huma.Error500InternalServerError("removing project: " + err.Error())
		}

		return nil, nil
	}
}

func updateProject(pm *engine.ProjectManager) func(context.Context, *ProjectPathInput) (*UpdateProjectOutput, error) {
	return func(ctx context.Context, input *ProjectPathInput) (*UpdateProjectOutput, error) {
		if _, err := pm.ResolveSpec(input.Name); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}

		if err := pm.Deploy(ctx, input.Name); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}

		return &UpdateProjectOutput{Body: struct {
			Name   string `json:"name" doc:"Project name"`
			Status string `json:"status" doc:"Update result"`
		}{Name: input.Name, Status: "deployed"}}, nil
	}
}

func nameFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	base := filepath.Base(u.Path)
	base = strings.TrimSuffix(base, ".git")
	if base == "" || base == "." {
		return "", fmt.Errorf("cannot derive project name from URL")
	}
	return base, nil
}
