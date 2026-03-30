package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/oreforge/ore/internal/config"
)

func ListProjects(cfg *config.OredConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		entries, err := os.ReadDir(cfg.Projects)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "reading projects directory: "+err.Error())
			return
		}

		var projects []string
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			specFile := filepath.Join(cfg.Projects, e.Name(), "ore.yaml")
			if _, statErr := os.Stat(specFile); statErr == nil {
				projects = append(projects, e.Name())
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
	}
}
