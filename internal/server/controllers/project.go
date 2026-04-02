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
}

func (rs ProjectResource) MountRoutes(s *fuego.Server) {
	projects := fuego.Group(s, "/projects")

	fuego.Get(projects, "/", rs.list,
		option.Summary("List projects"),
		option.Description("Returns names of all projects that contain an ore.yaml."),
		option.Tags("Projects"),
		option.OperationID("listProjects"),
	)
	fuego.Post(projects, "/", rs.add,
		option.Summary("Clone a project"),
		option.Description("Clones a git repository into the projects directory. The repository must contain an ore.yaml."),
		option.Tags("Projects"),
		option.OperationID("addProject"),
		option.DefaultStatusCode(http.StatusCreated),
		option.AddResponse(http.StatusConflict, "Project already exists", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusUnprocessableEntity, "Clone failed or missing ore.yaml", fuego.Response{Type: fuego.HTTPError{}}),
	)
	fuego.Delete(projects, "/{name}", rs.remove,
		option.Summary("Remove a project"),
		option.Description("Stops all containers and removes the project directory."),
		option.Tags("Projects"),
		option.OperationID("removeProject"),
		option.Path("name", "Project name"),
	)
	fuego.Patch(projects, "/{name}", rs.update,
		option.Summary("Update a project"),
		option.Description("Pulls latest changes from git and redeploys the project."),
		option.Tags("Projects"),
		option.OperationID("updateProject"),
		option.Path("name", "Project name"),
	)

	ops := fuego.Group(projects, "/{name}",
		option.Path("name", "Project name"),
	)

	fuego.Get(ops, "/status", rs.status,
		option.Summary("Get network status"),
		option.Description("Returns the status of all containers in the project network."),
		option.Tags("Projects"),
		option.OperationID("getStatus"),
	)
	fuego.PostStd(ops, "/up", rs.up,
		option.Summary("Start the network"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("up"),
		option.RequestBody(fuego.RequestBody{Type: dto.UpRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
	)
	fuego.PostStd(ops, "/down", rs.down,
		option.Summary("Stop the network"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("down"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
	)
	fuego.PostStd(ops, "/build", rs.build,
		option.Summary("Build images"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("build"),
		option.RequestBody(fuego.RequestBody{Type: dto.BuildRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
	)
	fuego.PostStd(ops, "/prune", rs.prune,
		option.Summary("Prune resources"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("prune"),
		option.RequestBody(fuego.RequestBody{Type: dto.PruneRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
	)
	fuego.PostStd(ops, "/clean", rs.clean,
		option.Summary("Clean artifacts"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Projects"),
		option.OperationID("clean"),
		option.RequestBody(fuego.RequestBody{Type: dto.CleanRequest{}}),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
	)
	fuego.GetStd(ops, "/console", rs.console,
		option.Summary("Server console"),
		option.Description("WebSocket endpoint for interactive server console. Send initial {width, height} JSON message after connecting."),
		option.Tags("Projects"),
		option.OperationID("console"),
		option.Query("server", "Container name to attach to"),
	)
}

func (rs ProjectResource) list(c fuego.ContextNoBody) (dto.ProjectListResponse, error) {
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

	name := body.Name
	if name == "" {
		var parseErr error
		name, parseErr = nameFromURL(body.URL)
		if parseErr != nil {
			return dto.ProjectResponse{}, fuego.HTTPError{Status: 400, Detail: "invalid repository URL: " + parseErr.Error()}
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

	return dto.ProjectResponse{Name: name}, nil
}

func (rs ProjectResource) remove(c fuego.ContextNoBody) (any, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return nil, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	_ = rs.PM.Down(c.Context(), name, slog.Default())

	projectDir := filepath.Join(rs.PM.ProjectsDir(), name)
	if err := os.RemoveAll(projectDir); err != nil {
		return nil, fuego.HTTPError{Status: 500, Detail: "removing project: " + err.Error()}
	}

	c.Response().WriteHeader(204)
	return nil, nil
}

func (rs ProjectResource) update(c fuego.ContextNoBody) (dto.UpdateProjectResponse, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.UpdateProjectResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	if err := rs.PM.Deploy(c.Context(), name); err != nil {
		return dto.UpdateProjectResponse{}, fuego.HTTPError{Status: 500, Detail: err.Error()}
	}

	return dto.UpdateProjectResponse{Name: name, Status: "deployed"}, nil
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
	s, err := rs.PM.Status(c.Context(), name)
	if err != nil {
		return dto.StatusResponse{}, fuego.HTTPError{Status: 500, Detail: err.Error()}
	}
	return *s, nil
}

func (rs ProjectResource) up(w http.ResponseWriter, r *http.Request) {
	name, err := rs.resolveProject(r)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	var body dto.UpRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	streamOperation(w, rs.LogLevel, func(logger *slog.Logger) error {
		return rs.PM.Up(r.Context(), name, project.UpOptions{
			NoCache: body.NoCache,
			Force:   body.Force,
		}, logger)
	})
}

func (rs ProjectResource) down(w http.ResponseWriter, r *http.Request) {
	name, err := rs.resolveProject(r)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, func(logger *slog.Logger) error {
		return rs.PM.Down(r.Context(), name, logger)
	})
}

func (rs ProjectResource) build(w http.ResponseWriter, r *http.Request) {
	name, err := rs.resolveProject(r)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	var body dto.BuildRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	streamOperation(w, rs.LogLevel, func(logger *slog.Logger) error {
		return rs.PM.Build(r.Context(), name, body.NoCache, logger)
	})
}

func (rs ProjectResource) prune(w http.ResponseWriter, r *http.Request) {
	name, err := rs.resolveProject(r)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	var body dto.PruneRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	var target project.PruneTarget
	switch body.Target {
	case "containers":
		target = project.PruneContainers
	case "images":
		target = project.PruneImages
	case "volumes":
		target = project.PruneVolumes
	default:
		target = project.PruneAll
	}
	streamOperation(w, rs.LogLevel, func(logger *slog.Logger) error {
		return rs.PM.Prune(r.Context(), name, target, logger)
	})
}

func (rs ProjectResource) clean(w http.ResponseWriter, r *http.Request) {
	name, err := rs.resolveProject(r)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	var body dto.CleanRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
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
	streamOperation(w, rs.LogLevel, func(logger *slog.Logger) error {
		return rs.PM.Clean(r.Context(), name, target, logger)
	})
}

func (rs ProjectResource) console(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("server")
	if serverName == "" {
		errs.Write(w, http.StatusBadRequest, "missing server query parameter")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	logger := slog.Default()

	_, msg, err := conn.Read(r.Context())
	if err != nil {
		logger.Error("console: reading initial size", "error", err)
		return
	}
	var size struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	if err := json.Unmarshal(msg, &size); err != nil {
		logger.Error("console: parsing initial size", "error", err)
		return
	}

	hijacked, err := rs.DockerClient.ContainerAttach(r.Context(), serverName, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		logger.Error("console: attaching to container", "container", serverName, "error", err)
		return
	}
	defer hijacked.Close()

	if size.Width > 0 && size.Height > 0 {
		_ = rs.DockerClient.ContainerResize(r.Context(), serverName, container.ResizeOptions{
			Width:  uint(size.Width),
			Height: uint(size.Height),
		})
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

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
				_ = conn.Close(websocket.StatusNormalClosure, "container process exited")
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			typ, data, readErr := conn.Read(ctx)
			if readErr != nil {
				_ = hijacked.CloseWrite()
				return
			}
			if typ == websocket.MessageText {
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
