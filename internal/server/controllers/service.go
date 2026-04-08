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

type ServiceResource struct {
	PM       *project.Manager
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
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Services"),
		option.OperationID("startService"),
		option.Path("service", "Service name"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Service not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(services, "/{service}/stop", rs.stop,
		option.Summary("Stop a service"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Services"),
		option.OperationID("stopService"),
		option.Path("service", "Service name"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Service not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(services, "/{service}/restart", rs.restart,
		option.Summary("Restart a service"),
		option.OverrideDescription(ndjsonDesc),
		option.Tags("Services"),
		option.OperationID("restartService"),
		option.Path("service", "Service name"),
		option.AddResponse(http.StatusOK, "NDJSON progress stream", fuego.Response{Type: dto.StreamLine{}}),
		option.AddResponse(http.StatusNotFound, "Service not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs ServiceResource) list(c fuego.ContextNoBody) (dto.ServiceListResponse, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.ServiceListResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	status, err := rs.PM.Status(c.Context(), name)
	if err != nil {
		return dto.ServiceListResponse{}, fuego.HTTPError{Status: 500, Detail: err.Error()}
	}

	return dto.ServiceListResponse{
		Services: status.Services,
	}, nil
}

func (rs ServiceResource) get(c fuego.ContextNoBody) (dto.ServiceStatusResponse, error) {
	name := c.PathParam("name")
	serviceName := c.PathParam("service")

	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.ServiceStatusResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	status, err := rs.PM.ServiceStatus(c.Context(), name, serviceName)
	if err != nil {
		return dto.ServiceStatusResponse{}, fuego.HTTPError{Status: 404, Detail: err.Error()}
	}

	return *status, nil
}

func (rs ServiceResource) start(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serviceName := r.PathValue("service")

	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return rs.PM.StartService(r.Context(), name, serviceName, logger)
	})
}

func (rs ServiceResource) stop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serviceName := r.PathValue("service")

	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return rs.PM.StopService(r.Context(), name, serviceName, logger)
	})
}

func (rs ServiceResource) restart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	serviceName := r.PathValue("service")

	if _, err := rs.PM.Resolve(name); err != nil {
		errs.Write(w, http.StatusNotFound, err.Error())
		return
	}

	streamOperation(w, rs.LogLevel, rs.Logger, func(logger *slog.Logger) error {
		return rs.PM.RestartService(r.Context(), name, serviceName, logger)
	})
}
