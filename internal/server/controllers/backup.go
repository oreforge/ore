package controllers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"github.com/oreforge/ore/internal/backup"
	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/project"
	"github.com/oreforge/ore/internal/server/dto"
	"github.com/oreforge/ore/internal/server/errs"
)

type BackupResource struct {
	PM       *project.Manager
	Backups  *backup.Service
	Store    *operation.Store
	LogLevel slog.Level
	Logger   *slog.Logger
}

func (rs BackupResource) MountRoutes(s *fuego.Server) {
	bearer := option.Security(openapi3.SecurityRequirement{"bearerAuth": {}})

	bs := fuego.Group(s, "/backups")

	fuego.Get(bs, "", rs.list,
		option.Summary("List backups"),
		option.Description("Returns all backups that belong to this project."),
		option.Tags("Backups"),
		option.OperationID("listBackups"),
		option.Query("volume", "Filter by docker volume name"),
		option.Query("status", "Filter by backup status"),
		option.AddResponse(http.StatusNotFound, "Project not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(bs, "", rs.create,
		option.Summary("Create a backup"),
		option.Description("Snapshots a volume into an archive stored locally. Returns an operation."),
		option.Tags("Backups"),
		option.OperationID("createBackup"),
		option.RequestBody(fuego.RequestBody{Type: dto.CreateBackupRequest{}}),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusBadRequest, "Invalid request body", fuego.Response{Type: fuego.HTTPError{}}),
		option.AddResponse(http.StatusConflict, "Operation already in progress", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.Get(bs, "/{id}", rs.get,
		option.Summary("Get backup detail"),
		option.Tags("Backups"),
		option.OperationID("getBackup"),
		option.Path("id", "Backup ID (ULID)"),
		option.AddResponse(http.StatusNotFound, "Backup not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.DeleteStd(bs, "/{id}", rs.delete,
		option.Summary("Delete a backup"),
		option.Description("Removes the archive and sidecar. Does not affect the source volume."),
		option.Tags("Backups"),
		option.OperationID("deleteBackup"),
		option.Path("id", "Backup ID"),
		option.DefaultStatusCode(http.StatusNoContent),
		option.AddResponse(http.StatusNotFound, "Backup not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(bs, "/{id}/restore", rs.restore,
		option.Summary("Restore a backup into its source volume"),
		option.Description("Extracts the archive into the source volume. Optionally takes a safety snapshot beforehand."),
		option.Tags("Backups"),
		option.OperationID("restoreBackup"),
		option.Path("id", "Backup ID"),
		option.Query("keep_pre_restore", "Take a pre-restore safety snapshot before extracting (default false)"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Backup not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
	fuego.PostStd(bs, "/{id}/verify", rs.verify,
		option.Summary("Verify backup integrity"),
		option.Description("Recomputes the sha256 of the archive and updates the stored verification timestamp on match."),
		option.Tags("Backups"),
		option.OperationID("verifyBackup"),
		option.Path("id", "Backup ID"),
		option.DefaultStatusCode(http.StatusAccepted),
		option.AddResponse(http.StatusAccepted, "Operation accepted", fuego.Response{Type: dto.OperationResponse{}}),
		option.AddResponse(http.StatusNotFound, "Backup not found", fuego.Response{Type: fuego.HTTPError{}}),
		bearer,
	)
}

func (rs BackupResource) list(c fuego.ContextNoBody) (dto.BackupListResponse, error) {
	projectName := c.PathParam("name")
	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		return dto.BackupListResponse{}, err
	}

	filter := backup.Filter{
		Project: networkName,
		Volume:  c.QueryParam("volume"),
		Status:  backup.Status(c.QueryParam("status")),
	}
	return dto.NewBackupListResponse(rs.Backups.List(filter)), nil
}

func (rs BackupResource) get(c fuego.ContextNoBody) (dto.BackupResponse, error) {
	projectName := c.PathParam("name")
	id := c.PathParam("id")
	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		return dto.BackupResponse{}, err
	}
	b, err := rs.lookupScoped(id, networkName)
	if err != nil {
		return dto.BackupResponse{}, err
	}
	return dto.NewBackupResponse(b), nil
}

func (rs BackupResource) lookupScoped(id, networkName string) (*backup.Backup, error) {
	b, err := rs.Backups.Get(id)
	if err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			return nil, fuego.HTTPError{Status: http.StatusNotFound, Detail: "backup not found"}
		}
		rs.Logger.Error("failed to get backup", "id", id, "error", err)
		return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "failed to get backup"}
	}
	if b.Project != networkName {
		return nil, fuego.HTTPError{Status: http.StatusNotFound, Detail: "backup not found"}
	}
	return b, nil
}

func (rs BackupResource) create(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")

	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		writeHTTPError(w, err)
		return
	}

	body, err := decodeBody[dto.CreateBackupRequest](r)
	if err != nil {
		errs.Write(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.Volume == "" {
		errs.Write(w, http.StatusBadRequest, "volume is required")
		return
	}

	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, operation.ActionBackupCreate, body.Volume,
		func(ctx context.Context, logger *slog.Logger) error {
			_, err := rs.Backups.Create(ctx, logger, backup.CreateOptions{
				Project: networkName,
				Volume:  body.Volume,
				Kind:    backup.KindManual,
				Tags:    body.Tags,
			})
			return err
		})
}

func (rs BackupResource) delete(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")
	id := r.PathValue("id")

	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	if _, err := rs.lookupScoped(id, networkName); err != nil {
		writeHTTPError(w, err)
		return
	}

	if err := rs.Backups.Remove(r.Context(), id); err != nil {
		rs.Logger.Error("failed to delete backup", "id", id, "error", err)
		errs.Write(w, http.StatusInternalServerError, "failed to delete backup")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rs BackupResource) restore(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")
	id := r.PathValue("id")
	keepPreRestore, _ := strconv.ParseBool(r.URL.Query().Get("keep_pre_restore"))

	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	if _, err := rs.lookupScoped(id, networkName); err != nil {
		writeHTTPError(w, err)
		return
	}

	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, operation.ActionBackupRestore, id,
		func(ctx context.Context, logger *slog.Logger) error {
			return rs.Backups.Restore(ctx, logger, id, backup.RestoreOptions{KeepPreRestore: keepPreRestore})
		})
}

func (rs BackupResource) verify(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("name")
	id := r.PathValue("id")

	networkName, err := resolveNetwork(rs.PM, rs.Logger, projectName)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	b, err := rs.lookupScoped(id, networkName)
	if err != nil {
		writeHTTPError(w, err)
		return
	}

	submitOperation(w, rs.PM, rs.Store, rs.Logger, rs.LogLevel, projectName, operation.ActionBackupVerify, id,
		func(ctx context.Context, logger *slog.Logger) error {
			res, err := rs.Backups.Verify(ctx, id)
			if err != nil {
				return err
			}
			if !res.MatchesRef {
				logger.Error("backup checksum mismatch", "id", id, "expected", b.Checksum, "got", res.Checksum)
				return errors.New("checksum mismatch")
			}
			logger.Info("backup verified", "id", id, "bytes", res.SizeBytes)
			return nil
		})
}

func writeHTTPError(w http.ResponseWriter, err error) {
	var he fuego.HTTPError
	if errors.As(err, &he) {
		errs.Write(w, he.Status, he.Detail)
		return
	}
	errs.Write(w, http.StatusInternalServerError, err.Error())
}
