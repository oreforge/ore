package engine

import (
	"context"
	"fmt"

	"github.com/oreforge/ore/internal/orchestrator"
)

type Remote struct {
	addr    string
	token   string
	project string
}

func NewRemote(addr, token, project string) (*Remote, error) {
	return &Remote{
		addr:    addr,
		token:   token,
		project: project,
	}, nil
}

func (r *Remote) Up(_ context.Context, _ bool) error {
	return fmt.Errorf("not implemented")
}

func (r *Remote) Down(_ context.Context) error {
	return fmt.Errorf("not implemented")
}

func (r *Remote) Build(_ context.Context, _ bool) error {
	return fmt.Errorf("not implemented")
}

func (r *Remote) Status(_ context.Context) (*orchestrator.NetworkStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *Remote) Prune(_ context.Context, _ PruneTarget) error {
	return fmt.Errorf("not implemented")
}

func (r *Remote) Clean(_ context.Context, _ CleanTarget) error {
	return fmt.Errorf("not implemented")
}

func (r *Remote) Console(_ context.Context, _ string, _ int) error {
	return fmt.Errorf("not implemented")
}

func (r *Remote) Close() error {
	return nil
}

func (r *Remote) ListProjects(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
