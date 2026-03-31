package providers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/oreforge/ore/internal/resolver"
	"github.com/oreforge/ore/internal/resolver/providers/gate"
	"github.com/oreforge/ore/internal/resolver/providers/paper"
	"github.com/oreforge/ore/internal/resolver/providers/velocity"
	"github.com/oreforge/ore/internal/resolver/runtimes"
)

type Provider interface {
	Resolve(ctx context.Context, id string, platform resolver.Platform) (*resolver.Artifact, error)
	Names() []string
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
	for _, name := range p.Names() {
		r.providers[name] = p
	}
}

func (r *Registry) Resolve(ctx context.Context, id string, platform resolver.Platform) (*resolver.Artifact, error) {
	name, _, ok := resolver.ParseSoftwareID(id)
	if !ok {
		return nil, fmt.Errorf("invalid software ID %q: expected name:version", id)
	}

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown software %q (supported: %s)", name, strings.Join(r.supportedSoftware(), ", "))
	}

	return p.Resolve(ctx, id, platform)
}

func (r *Registry) supportedSoftware() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func NewDefault(logger *slog.Logger) *Registry {
	rr := runtimes.NewDefault()

	r := NewRegistry()
	r.Register(paper.New(rr))
	r.Register(velocity.New(rr))
	r.Register(gate.New(rr, logger))
	return r
}
