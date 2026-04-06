package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"github.com/docker/docker/api/types/container"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

type ProjectResource struct {
	PM           *project.Manager
	DockerClient docker.Client
	LogLevel     slog.Level
	Logger       *slog.Logger
}

func (rs ProjectResource) MountRoutes(s *fuego.Server) {
	projects := fuego.Group(s, "/projects")

	fuego.Get(projects, "", rs.list,
		option.Summary("List projects"),
		option.Description("Returns names of all projects that contain an ore.yaml."),
		option.Tags("Projects"),
		option.OperationID("listProjects"),
	)
	fuego.Post(projects, "", rs.add,
		option.Summary("Clone a project"),
		option.Description("Clones a git repository into the projects directory. The repository must contain an ore.yaml."),
		option.Tags("Projects"),
		option.OperationID("addProject"),
		option.DefaultStatusCode(http.StatusCreated),
		option.AddResponse(http.StatusConflict, "Project already exists", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusUnprocessableEntity, "Clone failed or missing ore.yaml", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.DeleteStd(projects, "/{name}", rs.remove,
		option.Summary("Remove a project"),
		option.Description("Stops all servers and removes the project directory."),
		option.Tags("Projects"),
		option.OperationID("removeProject"),
		option.Path("name", "Project name"),
		option.DefaultStatusCode(http.StatusNoContent),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusInternalServerError, "Removal failed", fuego.Response{Type: fuego.HTTPError{}}),
	)

	ops := fuego.Group(projects, "/{name}",
		option.Path("name", "Project name"),
	)

	fuego.PostStd(ops, "/update", rs.update,
		option.Summary("Update a project"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("updateProject"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.Get(ops, "/status", rs.status,
		option.Summary("Get network status"),
		option.Description("Returns the status of all servers in the project network."),
		option.Tags("Projects"),
		option.OperationID("getStatus"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.PostStd(ops, "/up", rs.up,
		option.Summary("Start all servers"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("up"),
		option.RequestBody(fuego.RequestBody{Type: dto.UpRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.PostStd(ops, "/down", rs.down,
		option.Summary("Stop all servers"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("down"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.PostStd(ops, "/build", rs.build,
		option.Summary("Build images"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("build"),
		option.RequestBody(fuego.RequestBody{Type: dto.BuildRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.PostStd(ops, "/prune", rs.prune,
		option.Summary("Clean resources"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("prune"),
		option.RequestBody(fuego.RequestBody{Type: dto.PruneRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.PostStd(ops, "/clean", rs.clean,
		option.Summary("Clean artifacts"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("clean"),
		option.RequestBody(fuego.RequestBody{Type: dto.CleanRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.GetStd(ops, "/console", rs.console,
		option.Summary("Server console"),
		option.Description("WebSocket endpoint for interactive server console. Terminal I/O uses binary frames; send JSON text frames with {width, height} to resize."),
		option.Tags("Projects"),
		option.OperationID("console"),
		option.Query("server", "Server name to attach to"),
		option.Query("cols", "Initial terminal width (default 80)"),
		option.Query("rows", "Initial terminal height (default 24)"),
		option.AddResponse(http.StatusBadRequest, "Missing server parameter", fuego.Response{Type: fuego.HTTPError{}}),
	)
}

func (rs ProjectResource) list(_ fuego.ContextNoBody) (dto.ProjectListResponse, error) {
	names, err := rs.PM.List()
	if err != nil {
		return dto.ProjectListResponse{}, fuego.HTTPError{Status: 500, Detail: err.Error()}
	}
	return dto.ProjectListResponse{Projects: names}, nil
}

func (rs ProjectResource) add(c fuego.ContextWithBody[dto.AddProjectRequest]) (dto.ProjectResponse, error) {
	body, err := c.Body()
	if err != nil {
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 400, Detail: err.Error()}
	}

	repoURL, parseErr := url.Parse(body.URL)
	if parseErr != nil || (repoURL.Scheme != "https" && repoURL.Scheme != "http") {
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 400, Detail: "only http and https repository URLs are supported"}
	}

	name := body.Name
	if name == "" {
		var nameErr error
		name, nameErr = nameFromURL(body.URL)
		if nameErr != nil {
			return dto.ProjectResponse{}, fuego.HTTPError{Status: 400, Detail: "invalid repository URL: " + nameErr.Error()}
		}
	}

	if filepath.Base(name) != name {
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 400, Detail: "invalid project name"}
	}

	projectDir := filepath.Join(rs.PM.ProjectsDir(), name)
	if _, statErr := os.Stat(projectDir); statErr == nil {
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 409, Detail: "project " + name + " already exists"}
	}

	cmd := exec.CommandContext(c.Context(), "git", "clone", body.URL, projectDir)
	if output, cloneErr := cmd.CombinedOutput(); cloneErr != nil {
		_ = os.RemoveAll(projectDir)
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 422, Detail: "git clone failed: " + strings.TrimSpace(string(output))}
	}

	specPath := filepath.Join(projectDir, "ore.yaml")
	if _, statErr := os.Stat(specPath); statErr != nil {
		_ = os.RemoveAll(projectDir)
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 422, Detail: "repository does not contain an ore.yaml"}
	}

	rs.PM.RestartProjectPoll(name)

	return dto.ProjectResponse{Name: name}, nil
}

func (rs ProjectResource) remove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	rs.PM.StopProjectPoll(name)
	if err := rs.PM.Down(r.Context(), name, rs.Logger); err != nil {
		rs.Logger.Warn("failed to stop servers during removal", "project", name, "error", err)
	}

	projectDir := filepath.Join(rs.PM.ProjectsDir(), name)
	if err := os.RemoveAll(projectDir); err != nil {
		errs.Write(w, http.StatusInternalServerError, "removing project: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (rs ProjectResource) update(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		if err := rs.PM.Pull(r.Context(), name); err != nil {
			return fmt.Errorf("pulling %s: %w", name, err)
		}
		logger.Info("pulled latest changes", "project", name)
		if err := rs.PM.Up(r.Context(), name, project.UpOptions{}, logger); err != nil {
			return err
		}
		rs.PM.RestartProjectPoll(name)
		return nil
	})
}

func (rs ProjectResource) resolveProject(r *http.Request) (string, error) {
	name := r.PathValue("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return "", err
	}
	return name, nil
}

func (rs ProjectResource) status(c fuego.ContextNoBody) (dto.StatusResponse, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.StatusResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}
	s, err := rs.PM.Status(c.Context(), name)
	if err != nil {
		return dto.StatusResponse{}, fuego.HTTPError{Status: 500, Detail: err.Error()}
	}
	return *s, nil
}

func decodeBody[T any](r *http.Request) (T, error) {
	var body T
	if r.Body != nil {
		ct := r.Header.Get("Content-Type")
		if ct != "" && !strings.HasPrefix(ct, "application/json") {
			return body, fmt.Errorf("unsupported content type: %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			return body, err
		}
	}
	return body, nil
}

func (rs ProjectResource) resolveAndStream(w http.ResponseWriter, r *http.Request, fn func(name string, logger *slog.Logger) error) {
	name, err := rs.resolveProject(r)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return fn(name, logger)
	})
}

func (rs ProjectResource) up(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[dto.UpRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	rs.resolveAndStream(w, r, func(name string, logger *slog.Logger) error {
		return rs.PM.Up(r.Context(), name, project.UpOptions{
			NoCache: body.NoCache,
			Force:   body.Force,
		}, logger)
	})
}

func (rs ProjectResource) down(w http.ResponseWriter, r *http.Request) {
	rs.resolveAndStream(w, r, func(name string, logger *slog.Logger) error {
		return rs.PM.Down(r.Context(), name, logger)
	})
}

func (rs ProjectResource) build(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[dto.BuildRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	rs.resolveAndStream(w, r, func(name string, logger *slog.Logger) error {
		return rs.PM.Build(r.Context(), name, body.NoCache, logger)
	})
}

func (rs ProjectResource) prune(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[dto.PruneRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	var target project.PruneTarget
	switch body.Target {
	case "servers":
		target = project.PruneContainers
	case "images":
		target = project.PruneImages
	case "data":
		target = project.PruneVolumes
	default:
		target = project.PruneAll
	}

	rs.resolveAndStream(w, r, func(name string, logger *slog.Logger) error {
		return rs.PM.Prune(r.Context(), name, target, logger)
	})
}

func (rs ProjectResource) clean(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[dto.CleanRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	var target project.CleanTarget
	switch body.Target {
	case "cache":
		target = project.CleanCache
	case "builds":
		target = project.CleanBuilds
	default:
		target = project.CleanAll
	}

	rs.resolveAndStream(w, r, func(name string, logger *slog.Logger) error {
		return rs.PM.Clean(r.Context(), name, target, logger)
	})
}

func (rs ProjectResource) console(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("server")
	if serverName == "" {
		errs.Write(w, http.StatusBadRequest, "missing server query parameter")
		return
	}

	cols, rows := 80, 24
	if v := r.URL.Query().Get("cols"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cols = n
		}
	}
	if v := r.URL.Query().Get("rows"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rows = n
		}
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(64 * 1024)

	logger := rs.Logger

	hijacked, err := rs.DockerClient.ContainerAttach(r.Context(), serverName, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		logger.Error("console: attaching to server", "server", serverName, "error", err)
		return
	}
	defer hijacked.Close()

	_ = rs.DockerClient.ContainerResize(r.Context(), serverName, container.ResizeOptions{
		Width:  uint(cols),
		Height: uint(rows),
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = hijacked.Conn.Close()
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 4096)
		for {
			n, readErr := hijacked.Conn.Read(buf)
			if n > 0 {
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				_ = conn.Close(websocket.StatusNormalClosure, "server process exited")
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			msgType, data, readErr := conn.Read(ctx)
			if readErr != nil {
				_ = hijacked.CloseWrite()
				return
			}
			if msgType == websocket.MessageText {
				var resize struct {
					Width  int `json:"width"`
					Height int `json:"height"`
				}
				if json.Unmarshal(data, &resize) == nil && resize.Width > 0 && resize.Height > 0 {
					_ = rs.DockerClient.ContainerResize(ctx, serverName, container.ResizeOptions{
						Width:  uint(resize.Width),
						Height: uint(resize.Height),
					})
				}
				continue
			}
			if _, err := io.Copy(hijacked.Conn, bytes.NewReader(data)); err != nil {
				return
			}
		}
	}()

	wg.Wait()
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
