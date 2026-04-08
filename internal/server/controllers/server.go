package controllers

import (
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

type ServerResource struct {
	PM       *project.Manager
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
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Servers"),
		option.OperationID("startServer"),
		option.Path("server", "Server or service name"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Server not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Server not deployed yet", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(servers, "/{server}/stop", rs.stop,
		option.Summary("Stop a server"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Servers"),
		option.OperationID("stopServer"),
		option.Path("server", "Server or service name"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Server not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(servers, "/{server}/restart", rs.restart,
		option.Summary("Restart a server"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Servers"),
		option.OperationID("restartServer"),
		option.Path("server", "Server or service name"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Server not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Server not deployed yet", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs ServerResource) list(c fuego.ContextNoBody) (dto.ServerListResponse, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.ServerListResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	status, err := rs.PM.Status(c.Context(), name)
	if err != nil {
		return dto.ServerListResponse{}, fuego.HTTPError{Status: 500, Detail: err.Error()}
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
		return dto.ServerStatusResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	status, err := rs.PM.ServerStatus(c.Context(), name, serverName)
	if err != nil {
		return dto.ServerStatusResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	return *status, nil
}

func (rs ServerResource) start(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serverName := r.PathValue("server")

	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return rs.PM.StartServer(r.Context(), name, serverName, logger)
	})
}

func (rs ServerResource) stop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serverName := r.PathValue("server")

	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return rs.PM.StopServer(r.Context(), name, serverName, logger)
	})
}

func (rs ServerResource) restart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serverName := r.PathValue("server")

	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return rs.PM.RestartServer(r.Context(), name, serverName, logger)
	})
}
