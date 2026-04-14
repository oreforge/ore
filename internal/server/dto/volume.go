package dto

import (
	"time"

	"github.com/oreforge/ore/internal/volumes"
)

type VolumeResponse struct {
	Name       string   `json:"name" doc:"Docker volume name"`
	Project    string   `json:"project" doc:"Owning project (ore.project label)"`
	Owner      string   `json:"owner" doc:"Container name this volume is attached to"`
	OwnerKind  string   `json:"ownerKind" doc:"server or service"`
	Logical    string   `json:"logical" doc:"Logical volume name as declared in ore.yaml"`
	Driver     string   `json:"driver"`
	Mountpoint string   `json:"mountpoint,omitempty"`
	CreatedAt  string   `json:"createdAt,omitempty" example:"2026-04-14T10:00:00Z"`
	SizeBytes  int64    `json:"sizeBytes" doc:"Size in bytes; -1 when not yet measured"`
	InUseBy    []string `json:"inUseBy" doc:"Container names currently mounting this volume"`
}

type VolumeListResponse struct {
	Volumes []VolumeResponse `json:"volumes"`
}

func NewVolumeResponse(v volumes.Volume) VolumeResponse {
	resp := VolumeResponse{
		Name:       v.Name,
		Project:    v.Project,
		Owner:      v.Owner,
		OwnerKind:  v.OwnerKind,
		Logical:    v.Logical,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		SizeBytes:  v.SizeBytes,
		InUseBy:    v.InUseBy,
	}
	if !v.CreatedAt.IsZero() {
		resp.CreatedAt = v.CreatedAt.UTC().Format(time.RFC3339)
	}
	if resp.InUseBy == nil {
		resp.InUseBy = []string{}
	}
	return resp
}

func NewVolumeListResponse(vs []volumes.Volume) VolumeListResponse {
	out := VolumeListResponse{Volumes: make([]VolumeResponse, len(vs))}
	for i, v := range vs {
		out.Volumes[i] = NewVolumeResponse(v)
	}
	return out
}
