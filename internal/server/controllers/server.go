package controllers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
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
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.ServerListResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	status, err := rs.PM.Status(c.Context(), name)
	if err != nil {
		rs.Logger.Error("failed to get server list", "project", name, "error", err)
		return dto.ServerListResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to get server status"}
	}

	return dto.ServerListResponse{
		Servers:  status.Servers,
		Services: status.Services,
	}, nil
}

func (rs ServerResource) get(c fuego.ContextNoBody) (dto.ServerStatusResponse, error) {
	name := c.PathParam("name")
	serverName := c.PathParam("server")

	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.ServerStatusResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	status, err := rs.PM.ServerStatus(c.Context(), name, serverName)
	if err != nil {
		return dto.ServerStatusResponse{}, fuego.HTTPError{Status: 404, Detail: "server not found"}
	}

	return *status, nil
}

func (rs ServerResource) submit(w http.ResponseWriter, r *http.Request, action string, fn func(ctx context.Context, name, serverName string, logger *slog.Logger) error) {
	name := r.PathValue("name")
	serverName := r.PathValue("server")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, name, action, serverName,
		func(ctx context.Context, logger *slog.Logger) error {
			return fn(ctx, name, serverName, logger)
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
