package software

import (
	"fmt"
	"sync"
)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]Provider)
)

func Register(name string, p Provider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	if p == nil {
		panic("software: Register called with nil provider for " + name)
	}
	if _, dup := providers[name]; dup {
		panic("software: Register called twice for provider " + name)
	}
	providers[name] = p
}

func Providers() []string {
	providersMu.RLock()
	defer providersMu.RUnlock()
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

func lookup(name string) (Provider, error) {
	providersMu.RLock()
	defer providersMu.RUnlock()
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", ErrUnknownSoftware, name, Providers())
	}
	return p, nil
}
