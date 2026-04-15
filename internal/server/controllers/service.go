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

type ServiceResource struct {
	PM       *project.Manager
	Store    *operation.Store
	LogLevel slog.Level
	Logger   *slog.Logger
}

func (rs ServiceResource) MountRoutes(s *fuego.Server) {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	services := fuego.Group(s, "/services")

	fuego.Get(services, "", rs.list,
		option.Summary("List services"),
		option.Description("Returns the status of all services in the project network."),
		option.Tags("Services"),
		option.OperationID("listServices"),
		bearer,
	)
	fuego.Get(services, "/{service}", rs.get,
		option.Summary("Get service status"),
		option.Description("Returns the status of a single service by name."),
		option.Tags("Services"),
		option.OperationID("getService"),
		option.Path("service", "Service name"),
		bearer,
	)

	fuego.PostStd(services, "/{service}/start", rs.start,
		option.Summary("Start a service"),
		option.Description("Starts a service. Returns an operation that can be tracked via the operations API."),
		option.Tags("Services"),
		option.OperationID("startService"),
		option.Path("service", "Service name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Service not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(services, "/{service}/stop", rs.stop,
		option.Summary("Stop a service"),
		option.Description("Stops a service. Returns an operation that can be tracked via the operations API."),
		option.Tags("Services"),
		option.OperationID("stopService"),
		option.Path("service", "Service name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Service not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(services, "/{service}/restart", rs.restart,
		option.Summary("Restart a service"),
		option.Description("Restarts a service. Returns an operation that can be tracked via the operations API."),
		option.Tags("Services"),
		option.OperationID("restartService"),
		option.Path("service", "Service name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Service not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs ServiceResource) list(c fuego.ContextNoBody) (dto.ServiceListResponse, error) {
	projectName := c.PathParam("name")
	if _, err := rs.PM.Resolve(projectName); err != nil {
		return dto.ServiceListResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	status, err := rs.PM.Status(c.Context(), projectName)
	if err != nil {
		rs.Logger.Error("failed to get service list", "project", projectName, "error", err)
		return dto.ServiceListResponse{}, fuego.HTTPError{Status: 500, Detail: "failed to get service status"}
	}

	return dto.ServiceListResponse{
		Services: status.Services,
	}, nil
}

func (rs ServiceResource) get(c fuego.ContextNoBody) (dto.ServiceStatusResponse, error) {
	projectName := c.PathParam("name")
	serviceName := c.PathParam("service")

	if _, err := rs.PM.Resolve(projectName); err != nil {
		return dto.ServiceStatusResponse{}, fuego.HTTPError{Status: 404, Detail: "project not found"}
	}

	status, err := rs.PM.ServiceStatus(c.Context(), projectName, serviceName)
	if err != nil {
		return dto.ServiceStatusResponse{}, fuego.HTTPError{Status: 404, Detail: "service not found"}
	}

	return *status, nil
}

func (rs ServiceResource) submit(w http.ResponseWriter, r *http.Request, action string, fn func(ctx context.Context, projectName, serviceName string, logger *slog.Logger) error) {
	projectName := r.PathValue("name")
	serviceName := r.PathValue("service")
	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, action, serviceName,
		func(ctx context.Context, logger *slog.Logger) error {
			return fn(ctx, projectName, serviceName, logger)
		})
}

func (rs ServiceResource) start(w http.ResponseWriter, r *http.Request) {
	rs.submit(w, r, operation.ActionStart, rs.PM.StartService)
}

func (rs ServiceResource) stop(w http.ResponseWriter, r *http.Request) {
	rs.submit(w, r, operation.ActionStop, rs.PM.StopService)
}

func (rs ServiceResource) restart(w http.ResponseWriter, r *http.Request) {
	rs.submit(w, r, operation.ActionRestart, rs.PM.RestartService)
}
