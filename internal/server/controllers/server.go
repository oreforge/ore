package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

type ServerResource struct {
	PM       *project.Manager
	Store    *operation.Store
	LogLevel slog.Level
	Logger   *slog.Logger
}

func (rs ServerResource) MountRoutes(s *fuego.Server) {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	servers := fuego.Group(s, "/servers")

	fuego.Get(servers, "", rs.list,
		option.Summary("List servers"),
		option.Description("Returns the status of all servers and services in the project network."),
		option.Tags("Servers"),
		option.OperationID("listServers"),
		bearer,
	)
	fuego.Get(servers, "/{server}", rs.get,
		option.Summary("Get server status"),
		option.Description("Returns the status of a single server or service by name."),
		option.Tags("Servers"),
		option.OperationID("getServer"),
		option.Path("server", "Server or service name"),
		bearer,
	)

	fuego.PostStd(servers, "/{server}/start", rs.start,
		option.Summary("Start a server"),
		option.Description("Starts a server. Returns an operation that can be tracked via the operations API."),
		option.Tags("Servers"),
		option.OperationID("startServer"),
		option.Path("server", "Server or service name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Server not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(servers, "/{server}/stop", rs.stop,
		option.Summary("Stop a server"),
		option.Description("Stops a server. Returns an operation that can be tracked via the operations API."),
		option.Tags("Servers"),
		option.OperationID("stopServer"),
		option.Path("server", "Server or service name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Server not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(s, "/servers:batchStart", rs.batchStart,
		option.Summary("Batch start servers"),
		option.Description("Starts multiple servers in parallel. By default blocks and returns per-target results; pass ?async=true for the standard operation response."),
		option.Tags("Servers"),
		option.OperationID("batchStartServers"),
		option.RequestBody(fuego.RequestBody{Type: dto.BatchTargetsRequest{}}),
		option.Query("async", "Return 202 + operation response instead of waiting for completion"),
		option.AddResponse(http.StatusOK, "Batch completed", fuego.Response{Type: dto.BatchResponse{}}),
		option.AddResponse(http.StatusAccepted, "Operation accepted (async)", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request or unknown targets", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(s, "/servers:batchStop", rs.batchStop,
		option.Summary("Batch stop servers"),
		option.Description("Stops multiple servers in parallel. By default blocks and returns per-target results; pass ?async=true for the standard operation response."),
		option.Tags("Servers"),
		option.OperationID("batchStopServers"),
		option.RequestBody(fuego.RequestBody{Type: dto.BatchTargetsRequest{}}),
		option.Query("async", "Return 202 + operation response instead of waiting for completion"),
		option.AddResponse(http.StatusOK, "Batch completed", fuego.Response{Type: dto.BatchResponse{}}),
		option.AddResponse(http.StatusAccepted, "Operation accepted (async)", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request or unknown targets", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(s, "/servers:batchRestart", rs.batchRestart,
		option.Summary("Batch restart servers"),
		option.Description("Restarts multiple servers in parallel. By default blocks and returns per-target results; pass ?async=true for the standard operation response."),
		option.Tags("Servers"),
		option.OperationID("batchRestartServers"),
		option.RequestBody(fuego.RequestBody{Type: dto.BatchTargetsRequest{}}),
		option.Query("async", "Return 202 + operation response instead of waiting for completion"),
		option.AddResponse(http.StatusOK, "Batch completed", fuego.Response{Type: dto.BatchResponse{}}),
		option.AddResponse(http.StatusAccepted, "Operation accepted (async)", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request or unknown targets", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)

	fuego.PostStd(servers, "/{server}/restart", rs.restart,
		option.Summary("Restart a server"),
		option.Description("Restarts a server. Returns an operation that can be tracked via the operations API."),
		option.Tags("Servers"),
		option.OperationID("restartServer"),
		option.Path("server", "Server or service name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Server not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs ServerResource) list(c fuego.ContextNoBody) (dto.ServerListResponse, error) {
	projectName := c.PathParam("name")
	if _, err := rs.PM.Resolve(projectName); err != nil {
		return dto.ServerListResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	status, err := rs.PM.Status(c.Context(), projectName)
	if err != nil {
		rs.Logger.Error("failed to get server list", "project", projectName, "error", err)
		return dto.ServerListResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to get server status"}
	}

	return dto.ServerListResponse{
		Servers:  status.Servers,
		Services: status.Services,
	}, nil
}

func (rs ServerResource) get(c fuego.ContextNoBody) (dto.ServerStatusResponse, error) {
	projectName := c.PathParam("name")
	serverName := c.PathParam("server")

	if _, err := rs.PM.Resolve(projectName); err != nil {
		return dto.ServerStatusResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	status, err := rs.PM.ServerStatus(c.Context(), projectName, serverName)
	if err != nil {
		return dto.ServerStatusResponse{}, fuego.HTTPError{Status: 404, Detail: "server not found"}
	}

	return *status, nil
}

func (rs ServerResource) submit(w http.ResponseWriter, r *http.Request, action string, fn func(ctx context.Context, projectName, serverName string, logger *slog.Logger) error) {
	projectName := r.PathValue("name")
	serverName := r.PathValue("server")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, action, serverName,
		func(ctx context.Context, logger *slog.Logger) error {
			return fn(ctx, projectName, serverName, logger)
		})
}

func (rs ServerResource) start(w http.ResponseWriter, r *http.Request) {
	rs.submit(w, r, operation.ActionStart, rs.PM.StartServer)
}

func (rs ServerResource) stop(w http.ResponseWriter, r *http.Request) {
	rs.submit(w, r, operation.ActionStop, rs.PM.StopServer)
}

func (rs ServerResource) restart(w http.ResponseWriter, r *http.Request) {
	rs.submit(w, r, operation.ActionRestart, rs.PM.RestartServer)
}

func (rs ServerResource) batchStart(w http.ResponseWriter, r *http.Request) {
	rs.batchSubmit(w, r, operation.ActionBatchStart, rs.PM.StartServer)
}

func (rs ServerResource) batchStop(w http.ResponseWriter, r *http.Request) {
	rs.batchSubmit(w, r, operation.ActionBatchStop, rs.PM.StopServer)
}

func (rs ServerResource) batchRestart(w http.ResponseWriter, r *http.Request) {
	rs.batchSubmit(w, r, operation.ActionBatchRestart, rs.PM.RestartServer)
}

func (rs ServerResource) batchSubmit(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	fn func(ctx context.Context, projectName, targetName string, logger *slog.Logger) error,
) {
	projectName := r.PathValue("name")

	var req dto.BatchTargetsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	targets, err := normalizeTargets(req.Targets)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, rerr := rs.PM.Resolve(projectName); rerr != nil {
		errs.Write(w, http.StatusNotFound, "project not found")
		return
	}

	if missing := rs.findMissingServers(r.Context(), projectName, targets); len(missing) > 0 {
		errs.Write(w, http.StatusBadRequest, "unknown targets: "+strings.Join(missing, ","))
		return
	}

	submitBatchOperation(w, r, rs.Store, rs.Logger, rs.LogLevel,
		projectName, action, fmt.Sprintf("%d servers", len(targets)), targets,
		func(ctx context.Context, t string, l *slog.Logger) error {
			return fn(ctx, projectName, t, l)
		})
}

func (rs ServerResource) findMissingServers(ctx context.Context, projectName string, targets []string) []string {
	var missing []string
	for _, t := range targets {
		if _, err := rs.PM.ServerStatus(ctx, projectName, t); err != nil {
			missing = append(missing, t)
		}
	}
	return missing
}
