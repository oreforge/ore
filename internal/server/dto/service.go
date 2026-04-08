package dto

import "github.com/oreforge/ore/internal/deploy"

type ServiceListResponse struct {
	Services []deploy.ServerStatus `json:"services" doc:"Status of each service"`
}

type ServiceStatusResponse = deploy.ServerStatus
