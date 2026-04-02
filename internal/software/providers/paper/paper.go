package paper

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oreforge/ore/internal/software"
)

var _ software.Provider = (*Provider)(nil)

func New() *Provider {
	return &Provider{}
}

type Provider struct{}

func majorForMC(mcVersion string) int {
	parts := strings.SplitN(mcVersion, ".", 3)
	if len(parts) < 2 {
		return 21
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 21
	}
	switch {
	case minor >= 21:
		return 21
	case minor >= 17:
		return 17
	default:
		return 11
	}
}

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
	url := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds", version)
	var resp buildsResponse
	if err := software.GetJSON(ctx, url, &resp); err != nil {
		return nil, err
	}
	if len(resp.Builds) == 0 {
		return nil, fmt.Errorf("no builds found for paper %s", version)
	}
	latest := resp.Builds[len(resp.Builds)-1]
	downloadURL := fmt.Sprintf(
		"https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d/downloads/%s",
		version, latest.Build, latest.Downloads.Application.Name,
	)
	major := majorForMC(version)
	return &software.Artifact{
		Version: version,
		URL:     downloadURL,
		SHA256:  latest.Downloads.Application.Sha256,
		Runtime: software.Runtime{
			BaseImage:  fmt.Sprintf("eclipse-temurin:%d-jre-alpine", major),
			BinaryName: "server.jar",
			BinaryMode: 0o644,
			Entrypoint: "#!/bin/sh\necho \"eula=true\" > /data/eula.txt 2>/dev/null || true\nexec java $ORE_JVM_FLAGS -jar /opt/ore/server.jar \"$@\"\n",
			ExtraArgs:  "--nogui",
		},
		Health: software.HealthCheck{
			Timeout: 90 * time.Second,
			Retries: 30,
		},
	}, nil
}
