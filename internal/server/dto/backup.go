package dto

import (
	"time"

	"github.com/oreforge/ore/internal/backup"
)

type BackupResponse struct {
	ID          string             `json:"id"`
	Project     string             `json:"project"`
	Volume      string             `json:"volume"`
	LogicalName string             `json:"logicalName,omitempty"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status"`
	CreatedAt   string             `json:"createdAt"`
	FinishedAt  string             `json:"finishedAt,omitempty"`
	Error       string             `json:"error,omitempty"`
	SizeBytes   int64              `json:"sizeBytes"`
	Compressed  int64              `json:"compressed"`
	Algorithm   string             `json:"algorithm"`
	Checksum    string             `json:"checksum,omitempty"`
	Verified    string             `json:"verified,omitempty"`
	Storage     []BackupStorageRef `json:"storage"`
	Tags        []string           `json:"tags,omitempty"`
}

type BackupStorageRef struct {
	Backend string `json:"backend"`
	URI     string `json:"uri"`
}

type BackupListResponse struct {
	Backups []BackupResponse `json:"backups"`
}

type CreateBackupRequest struct {
	Volume string   `json:"volume" validate:"required"`
	Tags   []string `json:"tags,omitempty"`
}

type VerifyBackupResponse struct {
	ID         string `json:"id"`
	SizeBytes  int64  `json:"sizeBytes"`
	Checksum   string `json:"checksum"`
	MatchesRef bool   `json:"matchesRef"`
}

func NewBackupResponse(b *backup.Backup) BackupResponse {
	resp := BackupResponse{
		ID:          b.ID,
		Project:     b.Project,
		Volume:      b.Volume,
		LogicalName: b.LogicalName,
		Kind:        string(b.Kind),
		Status:      string(b.Status),
		CreatedAt:   b.CreatedAt.UTC().Format(time.RFC3339),
		Error:       b.Error,
		SizeBytes:   b.SizeBytes,
		Compressed:  b.Compressed,
		Algorithm:   b.Algorithm,
		Checksum:    b.Checksum,
		Tags:        b.Tags,
	}
	if b.FinishedAt != nil {
		resp.FinishedAt = b.FinishedAt.UTC().Format(time.RFC3339)
	}
	if b.Verified != nil {
		resp.Verified = b.Verified.UTC().Format(time.RFC3339)
	}
	for _, s := range b.Storage {
		resp.Storage = append(resp.Storage, BackupStorageRef{Backend: s.Backend, URI: s.URI})
	}
	if resp.Storage == nil {
		resp.Storage = []BackupStorageRef{}
	}
	return resp
}

func NewBackupListResponse(bs []*backup.Backup) BackupListResponse {
	out := BackupListResponse{Backups: make([]BackupResponse, 0, len(bs))}
	for _, b := range bs {
		out.Backups = append(out.Backups, NewBackupResponse(b))
	}
	return out
}
