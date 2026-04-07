package velocity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oreforge/ore/internal/software"
)

var _ software.Provider = (*Provider)(nil)

func New() *Provider {
	return &Provider{}
}

type Provider struct{}

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
	url := fmt.Sprintf("https://fill.papermc.io/v3/projects/velocity/versions/%s/builds", version)
	var builds []build
	if err := software.GetJSON(ctx, url, &builds); err != nil {
		return nil, err
	}
	if len(builds) == 0 {
		return nil, fmt.Errorf("no builds found for velocity %s", version)
	}
	selected, err := selectBuild(builds, channel)
	if err != nil {
		return nil, fmt.Errorf("velocity %s: %w", version, err)
	}
	dl, ok := selected.Downloads["server:default"]
	if !ok {
		return nil, fmt.Errorf("no server download found for velocity %s build %d", version, selected.ID)
	}
	return &software.Artifact{
		Version: version,
		URL:     dl.URL,
		SHA256:  dl.Checksums["sha256"],
		Runtime: software.Runtime{
			BaseImage:  "eclipse-temurin:21-jre-alpine",
			BinaryName: "server.jar",
			BinaryMode: 0o644,
			Entrypoint: "#!/bin/sh\necho \"eula=true\" > /data/eula.txt 2>/dev/null || true\nJAVA_OPTS=\"\"\n[ -n \"$ORE_MEMORY\" ] && JAVA_OPTS=\"-Xmx${ORE_MEMORY}\"\nexec java $JAVA_OPTS -jar /opt/ore/server.jar \"$@\"\n",
		},
		Health: software.HealthCheck{
			Timeout: 30 * time.Second,
			Retries: 15,
		},
	}, nil
}
