package dto

import (
	"time"

	"github.com/oreforge/ore/internal/operation"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

type OperationResponse struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

type OperationListResponse struct {
	Operations []OperationResponse `json:"operations"`
}

func NewOperationResponse(op *operation.Operation) OperationResponse {
	snap := op.Snapshot()
	return OperationResponse{
		ID:        snap.ID,
		Project:   snap.Project,
		Action:    snap.Action,
		Target:    snap.Target,
		Status:    string(snap.Status),
		Error:     snap.Error,
		CreatedAt: formatTime(snap.CreatedAt),
		StartedAt: formatTime(snap.StartedAt),
		EndedAt:   formatTime(snap.EndedAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeFormat)
}
