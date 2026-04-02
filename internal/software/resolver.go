package software

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/sync/singleflight"
)

type Resolver struct {
	flight singleflight.Group
}

func NewResolver() *Resolver {
	return &Resolver{}
}

func (r *Resolver) Resolve(ctx context.Context, spec string) (*Artifact, error) {
	name, version, err := ParseSpec(spec)
	if err != nil {
		return nil, err
	}

	result, err, _ := r.flight.Do(spec, func() (any, error) {
		p, err := lookup(name)
		if err != nil {
			return nil, err
		}
		return p.Resolve(ctx, version)
	})
	if err != nil {
		return nil, err
	}
	return result.(*Artifact), nil
}

func ParseSpec(spec string) (name, version string, err error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid software spec %q: must be name:version", spec)
	}
	return parts[0], parts[1], nil
}
