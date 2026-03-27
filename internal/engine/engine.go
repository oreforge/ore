package engine

import (
	"context"

	"github.com/oreforge/ore/internal/orchestrator"
)

type Engine interface {
	Up(ctx context.Context, specPath string, noCache bool) error
	Down(ctx context.Context, specPath string) error
	Build(ctx context.Context, specPath string, noCache bool) error
	Status(ctx context.Context, specPath string) (*orchestrator.NetworkStatus, error)
	Prune(ctx context.Context, specPath string, target PruneTarget) error
	Clean(ctx context.Context, specPath string, target CleanTarget) error
	Console(ctx context.Context, specPath string, serverName string, replica int) error
	Close() error
}

type PruneTarget int

const (
	PruneAll PruneTarget = iota
	PruneContainers
	PruneImages
	PruneVolumes
)

type CleanTarget int

const (
	CleanAll CleanTarget = iota
	CleanCache
	CleanBuilds
)
