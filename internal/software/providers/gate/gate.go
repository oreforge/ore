package gate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/oreforge/ore/internal/software"
)

var _ software.Provider = (*Provider)(nil)

func New() *Provider {
	return &Provider{}
}

type Provider struct{}

func getText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "oreforge/ore")
	resp, err := software.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func goarchToGate() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "386":
		return "386"
	default:
		return "amd64"
	}
}

func (p *Provider) Resolve(ctx context.Context, version string) (*software.Artifact, error) {
	arch := goarchToGate()
	filename := fmt.Sprintf("gate_%s_linux_%s", version, arch)
	downloadURL := fmt.Sprintf("https://github.com/minekube/gate/releases/download/v%s/%s", version, filename)

	checksumURL := fmt.Sprintf("https://github.com/minekube/gate/releases/download/v%s/checksums.txt", version)
	checksumText, err := getText(ctx, checksumURL)
	if err != nil {
		return nil, err
	}

	var sha256 string
	scanner := bufio.NewScanner(strings.NewReader(checksumText))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			sha256 = fields[0]
			break
		}
	}
	if sha256 == "" {
		return nil, fmt.Errorf("checksum not found for %s", filename)
	}

	return &software.Artifact{
		Version: version,
		URL:     downloadURL,
		SHA256:  sha256,
		Runtime: software.Runtime{
			BaseImage:  "alpine:latest",
			BinaryName: "server",
			BinaryMode: 0o755,
		},
		Health: software.HealthCheck{
			Timeout: 30 * time.Second,
			Retries: 15,
		},
	}, nil
}
