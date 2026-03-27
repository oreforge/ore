package runtimes

import (
	"github.com/oreforge/ore/internal/resolver/runtimes/java"
	"github.com/oreforge/ore/internal/resolver/runtimes/nativ"
)

type Runtime interface {
	Name() string
	BaseImage() string
	BinaryName() string
	Entrypoint() string
	BinaryMode() int64
}

type Registry struct {
	runtimes map[string]Runtime
}

func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[string]Runtime)}
}

func (r *Registry) Register(rt Runtime) {
	r.runtimes[rt.Name()] = rt
}

func (r *Registry) Get(name string) (Runtime, bool) {
	rt, ok := r.runtimes[name]
	return rt, ok
}

func NewDefault() *Registry {
	r := NewRegistry()
	r.Register(java.Runtime{Major: 11})
	r.Register(java.Runtime{Major: 17})
	r.Register(java.Runtime{Major: 21})
	r.Register(nativ.Runtime{})
	return r
}
