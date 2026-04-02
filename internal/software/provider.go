package software

import "context"

type Provider interface {
	Resolve(ctx context.Context, version string) (*Artifact, error)
}
