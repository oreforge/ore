package dto

import "github.com/oreforge/ore/internal/deploy"

type ServerListResponse struct {
	Servers  []deploy.ServerStatus `json:"servers" doc:"Status of each server"`
	Services []deploy.ServerStatus `json:"services,omitempty" doc:"Status of each service"`
}

type ServerStatusResponse = deploy.ServerStatus
