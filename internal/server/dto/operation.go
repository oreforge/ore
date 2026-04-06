package dto

import "github.com/oreforge/ore/internal/deploy"

type StatusResponse = deploy.NetworkStatus

type UpRequest struct {
	NoCache bool `json:"no_cache,omitempty" example:"false"`
	Force   bool `json:"force,omitempty" example:"false"`
}

type PruneRequest struct {
	Target string `json:"target,omitempty" example:"all" doc:"Target to clean (all, servers, images, data)"`
}

type CleanRequest struct {
	Target string `json:"target,omitempty" example:"all"`
}

type BuildRequest struct {
	NoCache bool `json:"no_cache,omitempty" example:"false"`
}
