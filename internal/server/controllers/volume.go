package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
	"github.com/oreforge/ore/internal/volumes"
)

type VolumeResource struct {
	PM       *project.Manager
	Volumes  *volumes.Service
	Store    *operation.Store
	LogLevel slog.Level
	Logger   *slog.Logger
}

func (rs VolumeResource) MountRoutes(s *fuego.Server) {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	vols := fuego.Group(s, "/volumes")

	fuego.Get(vols, "", rs.list,
		option.Summary("List volumes"),
		option.Description("Returns all ore-managed Docker volumes that belong to this project."),
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
	fuego.PostStd(vols, "/{volume}/measure", rs.measure,
		option.Summary("Measure volume size"),
		option.Description("Runs a helper container to compute the volume size. Returns an operation."),
		option.Tags("Volumes"),
		option.OperationID("measureVolume"),
		option.Path("volume", "Docker volume name"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Volume not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.DeleteStd(vols, "/{volume}", rs.delete,
		option.Summary("Delete a volume"),
		option.Description("Deletes an ore-managed Docker volume. Requires ?force=true if the volume is currently in use; that will stop the mounting containers first."),
		option.Tags("Volumes"),
		option.OperationID("deleteVolume"),
		option.Path("volume", "Docker volume name"),
		option.Query("force", "Force delete even if the volume is in use"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Volume not found", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Volume in use (retry with ?force=true)", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(vols, "/prune", rs.prune,
		option.Summary("Prune orphaned volumes"),
		option.Description("Deletes ore-managed volumes that are no longer declared in ore.yaml. Set ?dry_run=true to preview without deleting."),
		option.Tags("Volumes"),
		option.OperationID("pruneVolumes"),
		option.Query("dry_run", "Preview which volumes would be deleted without deleting them"),
		option.AddResponse(http.StatusOK, "Prune report", fuego.Response{Type: volumes.PruneReport{}}),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs VolumeResource) list(c fuego.ContextNoBody) (dto.VolumeListResponse, error) {
	projectName := c.PathParam("name")
	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		return dto.VolumeListResponse{}, err
	}

	vs, err := rs.Volumes.List(c.Context(), networkName)
	if err != nil {
		rs.Logger.Error("failed to list volumes", "project", projectName, "network", networkName, "error", err)
		return dto.VolumeListResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to list volumes"}
	}
	return dto.NewVolumeListResponse(vs), nil
}

func (rs VolumeResource) get(c fuego.ContextNoBody) (dto.VolumeResponse, error) {
	projectName := c.PathParam("name")
	volName := c.PathParam("volume")

	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		return dto.VolumeResponse{}, err
	}

	v, err := rs.Volumes.Inspect(c.Context(), volName)
	if err != nil {
		if errors.Is(err, volumes.ErrNotFound) {
			return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "volume not found"}
		}
		rs.Logger.Error("failed to inspect volume", "project", projectName, "volume", volName, "error", err)
		return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to inspect volume"}
	}

	if v.Project != networkName {
		return dto.VolumeResponse{}, fuego.HTTPError{Status: http.StatusNotFound, Detail: "volume not found"}
	}

	return dto.NewVolumeResponse(v), nil
}

func (rs VolumeResource) measure(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")
	volName := r.PathValue("volume")

	if err := rs.ensureVolumeInProject(r.Context(), w, projectName, volName); err != nil {
		return
	}

	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, operation.ActionVolumeMeasure, volName,
		func(ctx context.Context, logger *slog.Logger) error {
			size, err := rs.Volumes.Measure(ctx, volName)
			if err != nil {
				return err
			}
			logger.Info("volume measured", "volume", volName, "bytes", size)
			return nil
		})
}

func (rs VolumeResource) delete(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")
	volName := r.PathValue("volume")
	force, _ := strconv.ParseBool(r.URL.Query().Get("force"))

	if err := rs.ensureVolumeInProject(r.Context(), w, projectName, volName); err != nil {
		return
	}

	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, operation.ActionVolumeRemove, volName,
		func(ctx context.Context, logger *slog.Logger) error {
			if err := rs.Volumes.Remove(ctx, volName, force); err != nil {
				if errors.Is(err, volumes.ErrInUse) {
					logger.Warn("volume in use; pass force=true to stop containers and remove", "volume", volName, "error", err)
				}
				return err
			}
			logger.Info("volume removed", "volume", volName)
			return nil
		})
}

func (rs VolumeResource) prune(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")
	dryRun, _ := strconv.ParseBool(r.URL.Query().Get("dry_run"))

	s, err := loadNetworkSpec(rs.PM, rs.Logger, projectName)
	if err != nil {
		writeHTTPError(w, err)
		return
	}

	declared := deploy.DeclaredVolumeNames(s)

	report, err := rs.Volumes.Prune(r.Context(), s.Network, declared, dryRun)
	if err != nil {
		rs.Logger.Error("prune failed", "project", projectName, "network", s.Network, "error", err)
		errs.Write(w, http.StatusInternalServerError, "prune failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(report)
}

func (rs VolumeResource) ensureVolumeInProject(ctx context.Context, w http.ResponseWriter, projectName, volName string) error {
	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		writeHTTPError(w, err)
		return err
	}

	v, err := rs.Volumes.Inspect(ctx, volName)
	if err != nil {
		if errors.Is(err, volumes.ErrNotFound) {
			errs.Write(w, http.StatusNotFound, "volume not found")
			return err
		}
		rs.Logger.Error("failed to inspect volume", "project", projectName, "volume", volName, "error", err)
		errs.Write(w, http.StatusInternalServerError, "failed to inspect volume")
		return err
	}
	if v.Project != networkName {
		errs.Write(w, http.StatusNotFound, "volume not found")
		return errors.New("volume not in project")
	}
	return nil
}
