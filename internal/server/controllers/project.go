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
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
	"github.com/oreforge/ore/internal/spec"
	"github.com/oreforge/ore/internal/webhook"
)

type ProjectResource struct {
	PM           *project.Manager
	Store        *operation.Store
	DockerClient docker.Client
	LogLevel     slog.Level
	Logger       *slog.Logger
	Token        string
}

func (rs ProjectResource) MountRoutes(s *fuego.Server) *fuego.Server {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	projects := fuego.Group(s, "/projects")

	fuego.Get(projects, "", rs.list,
		option.Summary("List projects"),
		option.Description("Returns names of all projects that contain an ore.yaml."),
		option.Tags("Projects"),
		option.OperationID("listProjects"),
		bearer,
	)
	fuego.Post(projects, "", rs.add,
		option.Summary("Clone a project"),
		option.Description("Clones a git repository into the projects directory. The repository must contain an ore.yaml."),
		option.Tags("Projects"),
		option.OperationID("addProject"),
		option.DefaultStatusCode(http.StatusCreated),
		option.AddResponse(http.StatusConflict, "Project already exists", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusUnprocessableEntity, "Clone failed or missing ore.yaml", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.Get(projects, "/{name}", rs.detail,
		option.Summary("Get project detail"),
		option.Description("Returns the full project specification, deploy state, and gitops configuration."),
		option.Tags("Projects"),
		option.OperationID("getProject"),
		option.Path("name", "Project name"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.DeleteStd(projects, "/{name}", rs.remove,
		option.Summary("Remove a project"),
		option.Description("Stops all containers and removes the project directory."),
		option.Tags("Projects"),
		option.OperationID("removeProject"),
		option.Path("name", "Project name"),
		option.DefaultStatusCode(http.StatusNoContent),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusInternalServerError, "Removal failed", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)

	ops := fuego.Group(projects, "/{name}",
		option.Path("name", "Project name"),
	)

	fuego.Get(ops, "/builds", rs.builds,
		option.Summary("Get build history"),
		option.Description("Returns the build manifest including cached binaries and build artifacts."),
		option.Tags("Projects"),
		option.OperationID("getBuilds"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(ops, "/update", rs.update,
		option.Summary("Update a project"),
		option.Description("Pulls latest git changes and deploys. Returns an operation that can be tracked via the operations API."),
		option.Tags("Projects"),
		option.OperationID("updateProject"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.GetStd(ops, "/icon", rs.icon,
		option.Summary("Get project icon"),
		option.Description("Returns the project icon image file. Returns 404 if no icon is configured."),
		option.Tags("Projects"),
		option.OperationID("getProjectIcon"),
		option.AddResponse(http.StatusNotFound, "No icon configured or file not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.Get(ops, "/status", rs.status,
		option.Summary("Get network status"),
		option.Description("Returns the status of all servers in the project network."),
		option.Tags("Projects"),
		option.OperationID("getStatus"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(ops, "/up", rs.up,
		option.Summary("Start all servers"),
		option.Description("Builds and starts all servers. Returns an operation that can be tracked via the operations API."),
		option.Tags("Projects"),
		option.OperationID("up"),
		option.RequestBody(fuego.RequestBody{Type: dto.UpRequest{}}),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(ops, "/down", rs.down,
		option.Summary("Stop all servers"),
		option.Description("Stops all servers. Returns an operation that can be tracked via the operations API."),
		option.Tags("Projects"),
		option.OperationID("down"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(ops, "/build", rs.build,
		option.Summary("Build images"),
		option.Description("Builds Docker images. Returns an operation that can be tracked via the operations API."),
		option.Tags("Projects"),
		option.OperationID("build"),
		option.RequestBody(fuego.RequestBody{Type: dto.BuildRequest{}}),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(ops, "/clean", rs.clean,
		option.Summary("Clean resources"),
		option.Description("Cleans Docker resources. Returns an operation that can be tracked via the operations API."),
		option.Tags("Projects"),
		option.OperationID("clean"),
		option.RequestBody(fuego.RequestBody{Type: dto.CleanRequest{}}),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
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
		bearer,
	)
	fuego.Get(ops, "/webhook", rs.webhookInfo,
		option.Summary("Get webhook info"),
		option.Description("Returns the webhook URL and secret for this project. The secret is derived from HMAC-SHA256(token, project_name)."),
		option.Tags("Projects"),
		option.OperationID("getWebhookInfo"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)

	return ops
}

func (rs ProjectResource) list(_ fuego.ContextNoBody) (dto.ProjectListResponse, error) {
	names, err := rs.PM.List()
	if err != nil {
		rs.Logger.Error("failed to list projects", "error", err)
		return dto.ProjectListResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to list projects"}
	}
	return dto.ProjectListResponse{Projects: names}, nil
}

func (rs ProjectResource) detail(c fuego.ContextNoBody) (dto.ProjectDetailResponse, error) {
	name := c.PathParam("name")
	if _, resolveErr := rs.PM.Resolve(name); resolveErr != nil {
		return dto.ProjectDetailResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	_, s, state, err := rs.PM.Detail(name)
	if err != nil {
		rs.Logger.Error("failed to get project detail", "project", name, "error", err)
		return dto.ProjectDetailResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to load project details"}
	}

	return dto.ProjectDetailResponse{
		Name:  name,
		Spec:  dto.NewSpecResponse(s),
		State: dto.NewStateResponse(state),
	}, nil
}

func (rs ProjectResource) builds(c fuego.ContextNoBody) (dto.BuildsResponse, error) {
	name := c.PathParam("name")
	if _, resolveErr := rs.PM.Resolve(name); resolveErr != nil {
		return dto.BuildsResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	manifest, err := rs.PM.Builds(name)
	if err != nil {
		rs.Logger.Error("failed to get builds", "project", name, "error", err)
		return dto.BuildsResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to load build manifest"}
	}
	return *manifest, nil
}

var mimeTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
	".ico":  "image/x-icon",
}

func (rs ProjectResource) icon(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	iconPath, err := rs.PM.IconPath(name)
	if err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	f, err := os.Open(iconPath)
	if err != nil {
		errs.Write(w, http.StatusNotFound, "icon file not found")
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		errs.Write(w, http.StatusNotFound, "icon file not found")
		return
	}

	ext := strings.ToLower(filepath.Ext(iconPath))
	contentType, ok := mimeTypes[ext]
	if !ok {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
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
			return dto.ProjectResponse{}, fuego.HTTPError{Status: 400, Detail: "cannot derive project name from URL"}
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
		rs.Logger.Error("git clone failed", "url", body.URL, "output", strings.TrimSpace(string(output)))
		return dto.ProjectResponse{}, fuego.HTTPError{Status: 422, Detail: "git clone failed"}
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
		errs.Write(w, http.StatusNotFound, "project not found")
		return
	}

	rs.PM.StopProjectPoll(name)
	if err := rs.PM.Down(r.Context(), name, rs.Logger); err != nil {
		rs.Logger.Warn("failed to stop servers during removal", "project", name, "error", err)
	}

	projectDir := filepath.Join(rs.PM.ProjectsDir(), name)
	if err := os.RemoveAll(projectDir); err != nil {
		rs.Logger.Error("failed to remove project directory", "project", name, "error", err)
		errs.Write(w, http.StatusInternalServerError, "failed to remove project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (rs ProjectResource) update(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, name, operation.ActionUpdate, "",
		func(ctx context.Context, logger *slog.Logger) error {
			if err := rs.PM.Pull(ctx, name); err != nil {
				return fmt.Errorf("pulling %s: %w", name, err)
			}
			logger.Info("pulled latest changes", "project", name)
			if err := rs.PM.Up(ctx, name, project.UpOptions{}, logger); err != nil {
				return err
			}
			rs.PM.RestartProjectPoll(name)
			return nil
		})
}

func (rs ProjectResource) status(c fuego.ContextNoBody) (dto.StatusResponse, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.StatusResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}
	s, err := rs.PM.Status(c.Context(), name)
	if err != nil {
		rs.Logger.Error("failed to get status", "project", name, "error", err)
		return dto.StatusResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to get project status"}
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

func (rs ProjectResource) up(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[dto.UpRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	name := r.PathValue("name")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, name, operation.ActionUp, "",
		func(ctx context.Context, logger *slog.Logger) error {
			return rs.PM.Up(ctx, name, project.UpOptions{
				NoCache: body.NoCache,
				Force:   body.Force,
			}, logger)
		})
}

func (rs ProjectResource) down(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, name, operation.ActionDown, "",
		func(ctx context.Context, logger *slog.Logger) error {
			return rs.PM.Down(ctx, name, logger)
		})
}

func (rs ProjectResource) build(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[dto.BuildRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	name := r.PathValue("name")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, name, operation.ActionBuild, "",
		func(ctx context.Context, logger *slog.Logger) error {
			return rs.PM.Build(ctx, name, body.NoCache, logger)
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
	case "", "all":
		target = project.CleanAll
	case "containers":
		target = project.CleanContainers
	case "images":
		target = project.CleanImages
	case "volumes":
		target = project.CleanVolumes
	case "cache":
		target = project.CleanCache
	case "builds":
		target = project.CleanBuilds
	default:
		errs.Write(w, http.StatusBadRequest, "unknown clean target: "+body.Target+" (valid: all, containers, images, volumes, cache, builds)")
		return
	}

	name := r.PathValue("name")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, name, operation.ActionClean, "",
		func(ctx context.Context, logger *slog.Logger) error {
			return rs.PM.Clean(ctx, name, target, logger)
		})
}

func (rs ProjectResource) console(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serverName := r.URL.Query().Get("server")
	if serverName == "" {
		errs.Write(w, http.StatusBadRequest, "missing server query parameter")
		return
	}

	specPath, err := rs.PM.Resolve(name)
	if err != nil {
		errs.Write(w, http.StatusNotFound, "project not found")
		return
	}
	s, err := spec.Load(specPath)
	if err != nil {
		rs.Logger.Error("console: failed to load spec", "project", name, "error", err)
		errs.Write(w, http.StatusInternalServerError, "failed to load project spec")
		return
	}
	if !hasServer(s, serverName) {
		errs.Write(w, http.StatusBadRequest, "server not found in project spec")
		return
	}

	cols, err := parseTermDim(r, "cols", 80, 500)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := parseTermDim(r, "rows", 24, 200)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, err.Error())
		return
	}

	conn, err := websocket.Accept(w, r, nil)
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
						Width:  uint(min(resize.Width, 500)),
						Height: uint(min(resize.Height, 200)),
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

func parseTermDim(r *http.Request, param string, defaultVal, max int) (int, error) {
	v := r.URL.Query().Get(param)
	if v == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: must be a positive integer", param)
	}
	if n < 1 {
		return 1, nil
	}
	if n > max {
		return max, nil
	}
	return n, nil
}

func (rs ProjectResource) webhookInfo(c fuego.ContextNoBody) (dto.WebhookInfoResponse, error) {
	name := c.PathParam("name")
	specPath, err := rs.PM.Resolve(name)
	if err != nil {
		return dto.WebhookInfoResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	s, err := spec.Load(specPath)
	if err != nil {
		rs.Logger.Error("failed to load spec", "project", name, "error", err)
		return dto.WebhookInfoResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to load project spec"}
	}

	enabled := s.GitOps != nil && s.GitOps.Webhook.Enabled
	if !enabled {
		return dto.WebhookInfoResponse{Enabled: false}, nil
	}

	secret := webhook.DeriveSecret(rs.Token, name)
	webhookURL := fmt.Sprintf("/api/webhook/%s?secret=%s", url.PathEscape(name), secret)

	return dto.WebhookInfoResponse{
		Enabled: true,
		URL:     webhookURL,
		Secret:  secret,
		Force:   s.GitOps.Webhook.Force,
		NoCache: s.GitOps.Webhook.NoCache,
	}, nil
}

func hasServer(s *spec.Network, name string) bool {
	for _, srv := range s.Servers {
		if srv.Name == name {
			return true
		}
	}
	return false
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
