package providers

import (
	"sync"

	"github.com/oreforge/ore/internal/software"
	"github.com/oreforge/ore/internal/software/providers/gate"
	"github.com/oreforge/ore/internal/software/providers/paper"
	"github.com/oreforge/ore/internal/software/providers/velocity"
)

var once sync.Once

func New() *software.Resolver {
	once.Do(func() {
		software.Register("paper", paper.New())
		software.Register("velocity", velocity.New())
		software.Register("gate", gate.New())
	})
	return software.NewResolver()
}
