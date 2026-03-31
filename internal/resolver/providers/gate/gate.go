package gate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oreforge/ore/internal/resolver"
	"github.com/oreforge/ore/internal/resolver/runtimes"
)

type Gate struct {
	runtimes *runtimes.Registry
	logger   *slog.Logger
}

func New(runtimes *runtimes.Registry, logger *slog.Logger) *Gate {
	return &Gate{runtimes: runtimes, logger: logger}
}

func (g *Gate) Names() []string {
	return []string{"gate"}
}

func (g *Gate) Resolve(ctx context.Context, id string, platform resolver.Platform) (*resolver.Artifact, error) {
	_, version, _ := resolver.ParseSoftwareID(id)

	arch := platform.Arch
	filename := fmt.Sprintf("gate_%s_linux_%s", version, arch)
	downloadURL := fmt.Sprintf("https://github.com/minekube/gate/releases/download/v%s/%s", version, filename)
	checksumsURL := fmt.Sprintf("https://github.com/minekube/gate/releases/download/v%s/checksums.txt", version)

	sha256 := g.fetchChecksum(ctx, checksumsURL, filename)

	rt, ok := g.runtimes.Get("native")
	if !ok {
		return nil, fmt.Errorf("no native runtime registered")
	}

	return &resolver.Artifact{
		URL:           downloadURL,
		SHA256:        sha256,
		BuildID:       version,
		Runtime:       rt,
		HealthTimeout: 30 * time.Second,
		HealthRetries: 15,
	}, nil
}

func (g *Gate) fetchChecksum(ctx context.Context, checksumsURL, filename string) string {
	body, err := resolver.GetRaw(ctx, checksumsURL)
	if err != nil {
		g.logger.Warn("failed to fetch gate checksums", "url", checksumsURL, "error", err)
		return ""
	}

	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			return parts[0]
		}
	}

	g.logger.Warn("checksum not found for gate binary", "filename", filename)
	return ""
}
