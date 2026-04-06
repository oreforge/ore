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

type build struct {
	ID        int                 `json:"id"`
	Channel   string              `json:"channel"`
	Downloads map[string]download `json:"downloads"`
}

type download struct {
	Name      string            `json:"name"`
	Checksums map[string]string `json:"checksums"`
	URL       string            `json:"url"`
}

var channelMap = map[string]string{
	"experimental": "ALPHA",
	"beta":         "BETA",
	"stable":       "STABLE",
}

func parseVersionChannel(version string) (string, string) {
	if i := strings.LastIndex(version, "@"); i >= 0 {
		return version[:i], version[i+1:]
	}
	return version, ""
}

func selectBuild(builds []build, channel string) (build, error) {
	if channel == "" {
		return builds[len(builds)-1], nil
	}
	apiChannel, ok := channelMap[channel]
	if !ok {
		return build{}, fmt.Errorf("unknown channel %q (valid: experimental, beta, stable)", channel)
	}
	for i := len(builds) - 1; i >= 0; i-- {
		if builds[i].Channel == apiChannel {
			return builds[i], nil
		}
	}
	return build{}, fmt.Errorf("no %s channel build found", channel)
}

func (p *Provider) Resolve(ctx context.Context, version string) (*software.Artifact, error) {
	version, channel := parseVersionChannel(version)
	url := fmt.Sprintf("https://fill.papermc.io/v3/projects/paper/versions/%s/builds", version)
	var builds []build
	if err := software.GetJSON(ctx, url, &builds); err != nil {
		return nil, err
	}
	if len(builds) == 0 {
		return nil, fmt.Errorf("no builds found for paper %s", version)
	}
	selected, err := selectBuild(builds, channel)
	if err != nil {
		return nil, fmt.Errorf("paper %s: %w", version, err)
	}
	dl, ok := selected.Downloads["server:default"]
	if !ok {
		return nil, fmt.Errorf("no server download found for paper %s build %d", version, selected.ID)
	}
	major := majorForMC(version)
	return &software.Artifact{
		Version: version,
		URL:     dl.URL,
		SHA256:  dl.Checksums["sha256"],
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
