package paper

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/oreforge/ore/internal/resolver"
	"github.com/oreforge/ore/internal/resolver/runtimes"
	"github.com/oreforge/ore/internal/resolver/runtimes/java"
)

type Paper struct {
	runtimes *runtimes.Registry
}

func New(runtimes *runtimes.Registry) *Paper {
	return &Paper{runtimes: runtimes}
}

func (p *Paper) Names() []string {
	return []string{"paper"}
}

func (p *Paper) Resolve(ctx context.Context, id string, _ resolver.Platform) (*resolver.Artifact, error) {
	_, version, _ := resolver.ParseSoftwareID(id)

	artifact, err := p.resolveLatestBuild(ctx, version)
	if err != nil {
		return nil, err
	}

	rtName := "java:" + strconv.Itoa(java.MajorForMC(version))
	rt, ok := p.runtimes.Get(rtName)
	if !ok {
		return nil, fmt.Errorf("no %s runtime registered", rtName)
	}

	artifact.Runtime = rt
	artifact.ExtraArgs = "--nogui"
	artifact.HealthTimeout = 90 * time.Second
	artifact.HealthRetries = 30

	return artifact, nil
}

func (p *Paper) resolveLatestBuild(ctx context.Context, version string) (*resolver.Artifact, error) {
	buildsURL := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds", version)

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
		return nil, fmt.Errorf("fetching paper builds: %w", err)
	}

	if len(resp.Builds) == 0 {
		return nil, fmt.Errorf("no builds found for paper:%s", version)
	}

	latest := resp.Builds[len(resp.Builds)-1]
	app, ok := latest.Downloads["application"]
	if !ok {
		return nil, fmt.Errorf("no application download for paper:%s build %d", version, latest.Build)
	}

	return &resolver.Artifact{
		URL:     fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d/downloads/%s", version, latest.Build, app.Name),
		SHA256:  app.SHA256,
		BuildID: fmt.Sprintf("%d", latest.Build),
	}, nil
}
