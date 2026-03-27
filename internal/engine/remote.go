package engine

import (
	"context"
	"fmt"

	"github.com/oreforge/ore/internal/orchestrator"
)

type Remote struct {
	addr string
}

func NewRemote(addr string) *Remote {
	return &Remote{addr: addr}
}

func (r *Remote) Up(ctx context.Context, specPath string, noCache bool) error {
	return fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Down(ctx context.Context, specPath string) error {
	return fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Build(ctx context.Context, specPath string, noCache bool) error {
	return fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Status(ctx context.Context, specPath string) (*orchestrator.NetworkStatus, error) {
	return nil, fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Prune(ctx context.Context, specPath string, target PruneTarget) error {
	return fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Clean(ctx context.Context, specPath string, target CleanTarget) error {
	return fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Console(ctx context.Context, specPath string, serverName string, replica int) error {
	return fmt.Errorf("remote engine not implemented (addr: %s)", r.addr)
}

func (r *Remote) Close() error {
	return nil
}
