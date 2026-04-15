package controllers

import (
	"log/slog"
	"net/http"

	"github.com/go-fuego/fuego"

	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/spec"
)

func resolveNetwork(pm *project.Manager, logger *slog.Logger, projectName string) (string, error) {
	specPath, err := pm.Resolve(projectName)
	if err != nil {
		return "", fuego.HTTPError{Status: http.StatusNotFound, Detail: "project not found"}
	}
	s, err := spec.Load(specPath)
	if err != nil {
		logger.Error("failed to load project spec", "project", projectName, "error", err)
		return "", fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to load project spec"}
	}
	return s.Network, nil
}

func loadNetworkSpec(pm *project.Manager, logger *slog.Logger, projectName string) (*spec.Network, error) {
	specPath, err := pm.Resolve(projectName)
	if err != nil {
		return nil, fuego.HTTPError{Status: http.StatusNotFound, Detail: "project not found"}
	}
	s, err := spec.Load(specPath)
	if err != nil {
		logger.Error("failed to load project spec", "project", projectName, "error", err)
		return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to load project spec"}
	}
	return s, nil
}
