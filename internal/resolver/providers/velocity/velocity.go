package velocity

import (
	"context"
	"fmt"
	"time"

	"github.com/oreforge/ore/internal/resolver"
	"github.com/oreforge/ore/internal/resolver/runtimes"
)

type Velocity struct {
	runtimes *runtimes.Registry
}

func New(runtimes *runtimes.Registry) *Velocity {
	return &Velocity{runtimes: runtimes}
}

func (v *Velocity) Names() []string {
	return []string{"velocity"}
}

func (v *Velocity) Resolve(ctx context.Context, id string, _ resolver.Platform) (*resolver.Artifact, error) {
	_, version, _ := resolver.ParseSoftwareID(id)

	artifact, err := v.resolveLatestBuild(ctx, version)
	if err != nil {
		return nil, err
	}

	rt, ok := v.runtimes.Get("java:21")
	if !ok {
		return nil, fmt.Errorf("no java:21 runtime registered")
	}

	artifact.Runtime = rt
	artifact.HealthTimeout = 30 * time.Second
	artifact.HealthRetries = 15

	return artifact, nil
}

func (v *Velocity) resolveLatestBuild(ctx context.Context, version string) (*resolver.Artifact, error) {
	buildsURL := fmt.Sprintf("https://api.papermc.io/v2/projects/velocity/versions/%s/builds", version)

	var resp struct {
		Builds []struct {
			Build     int `json:"build"`
			Downloads map[string]struct {
				Name   string `json:"name"`
				SHA256 string `json:"sha256"`
			} `json:"downloads"`
		} `json:"builds"`
	}

	if err := resolver.GetJSON(ctx, buildsURL, &resp); err != nil {
		return nil, fmt.Errorf("fetching velocity builds: %w", err)
	}

	if len(resp.Builds) == 0 {
		return nil, fmt.Errorf("no builds found for velocity:%s", version)
	}

	latest := resp.Builds[len(resp.Builds)-1]
	app, ok := latest.Downloads["application"]
	if !ok {
		return nil, fmt.Errorf("no application download for velocity:%s build %d", version, latest.Build)
	}

	return &resolver.Artifact{
		URL:     fmt.Sprintf("https://api.papermc.io/v2/projects/velocity/versions/%s/builds/%d/downloads/%s", version, latest.Build, app.Name),
		SHA256:  app.SHA256,
		BuildID: fmt.Sprintf("%d", latest.Build),
	}, nil
}
