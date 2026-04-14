package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/volumes"
)

type VolumeResource struct {
	PM      *project.Manager
	Volumes *volumes.Service
	Logger  *slog.Logger
}

func (rs VolumeResource) MountRoutes(s *fuego.Server) {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	vols := fuego.Group(s, "/volumes")

	fuego.Get(vols, "", rs.list,
		option.Summary("List volumes"),
		option.Description("Returns all ore-managed Docker volumes that belong to this project. "+
			"Volumes are identified by the ore.managed=true label applied at create time; "+
			"pre-label volumes are not surfaced until relabelled."),
		option.Tags("Volumes"),
		option.OperationID("listVolumes"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.Get(vols, "/{volume}", rs.get,
		option.Summary("Get volume detail"),
		option.Description("Returns metadata and current usage for a single volume."),
		option.Tags("Volumes"),
		option.OperationID("getVolume"),
		option.Path("volume", "Docker volume name"),
		option.AddResponse(http.StatusNotFound, "Volume not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs VolumeResource) list(c fuego.ContextNoBody) (dto.VolumeListResponse, error) {
	name := c.PathParam("name")
	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.VolumeListResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "project not found"}
	}

	vs, err := rs.Volumes.List(c.Context(), name)
	if err != nil {
		rs.Logger.Error("failed to list volumes", "project", name, "error", err)
		return dto.VolumeListResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to list volumes"}
	}
	return dto.NewVolumeListResponse(vs), nil
}

func (rs VolumeResource) get(c fuego.ContextNoBody) (dto.VolumeResponse, error) {
	name := c.PathParam("name")
	volName := c.PathParam("volume")

	if _, err := rs.PM.Resolve(name); err != nil {
		return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "project not found"}
	}

	v, err := rs.Volumes.Inspect(c.Context(), volName)
	if err != nil {
		if errors.Is(err, volumes.ErrNotFound) {
			return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "volume not found"}
		}
		rs.Logger.Error("failed to inspect volume", "project", name, "volume", volName, "error", err)
		return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to inspect volume"}
	}

	if v.Project != name {
		return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "volume not found"}
	}

	return dto.NewVolumeResponse(v), nil
}
