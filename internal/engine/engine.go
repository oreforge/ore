package engine

import (
	"context"

	"github.com/oreforge/ore/internal/orchestrator"
)

type UpOptions struct {
	NoCache bool
	Force   bool
}

type Engine interface {
	Up(ctx context.Context, opts UpOptions) error
	Down(ctx context.Context) error
	Build(ctx context.Context, noCache bool) error
	Status(ctx context.Context) (*orchestrator.NetworkStatus, error)
	Prune(ctx context.Context, target PruneTarget) error
	Clean(ctx context.Context, target CleanTarget) error
	Console(ctx context.Context, serverName string) error
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
