package velocity

import (
	"context"
	"fmt"
	"time"

	"github.com/oreforge/ore/internal/software"
)

var _ software.Provider = (*Provider)(nil)

func New() *Provider {
	return &Provider{}
}

type Provider struct{}

type buildsResponse struct {
	Builds []struct {
		Build     int `json:"build"`
		Downloads struct {
			Application struct {
				Name   string `json:"name"`
				Sha256 string `json:"sha256"`
			} `json:"application"`
		} `json:"downloads"`
	} `json:"builds"`
}

func (p *Provider) Resolve(ctx context.Context, version string) (*software.Artifact, error) {
	url := fmt.Sprintf("https://api.papermc.io/v2/projects/velocity/versions/%s/builds", version)
	var resp buildsResponse
	if err := software.GetJSON(ctx, url, &resp); err != nil {
		return nil, err
	}
	if len(resp.Builds) == 0 {
		return nil, fmt.Errorf("no builds found for velocity %s", version)
	}
	latest := resp.Builds[len(resp.Builds)-1]
	downloadURL := fmt.Sprintf(
		"https://api.papermc.io/v2/projects/velocity/versions/%s/builds/%d/downloads/%s",
		version, latest.Build, latest.Downloads.Application.Name,
	)
	return &software.Artifact{
		Version: version,
		URL:     downloadURL,
		SHA256:  latest.Downloads.Application.Sha256,
		Runtime: software.Runtime{
			BaseImage:  "eclipse-temurin:21-jre-alpine",
			BinaryName: "server.jar",
			BinaryMode: 0o644,
			Entrypoint: "#!/bin/sh\necho \"eula=true\" > /data/eula.txt 2>/dev/null || true\nexec java $ORE_JVM_FLAGS -jar /opt/ore/server.jar \"$@\"\n",
		},
		Health: software.HealthCheck{
			Timeout: 30 * time.Second,
			Retries: 15,
		},
	}, nil
}
