package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/oreforge/ore/internal/resolver/runtimes"
)

type Platform struct {
	OS   string
	Arch string
}

type Artifact struct {
	URL           string
	SHA256        string
	BuildID       string
	Runtime       runtimes.Runtime
	ExtraArgs     string
	HealthTimeout time.Duration
	HealthRetries int
}

func ParseSoftwareID(id string) (name, version string, ok bool) {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			if i > 0 && i < len(id)-1 {
				return id[:i], id[i+1:], true
			}
			return "", "", false
		}
	}
	return "", "", false
}

func GetJSON(ctx context.Context, url string, target any) error {
	data, err := GetRaw(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func GetRaw(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oreforge/ore")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
