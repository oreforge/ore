package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/oreforge/ore/internal/server/dto"
)

func (c *Client) Volumes(ctx context.Context) ([]dto.VolumeResponse, error) {
	if err := c.requireProject(); err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, c.projectPath()+"/volumes", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("volumes request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var out dto.VolumeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding volumes: %w", err)
	}
	return out.Volumes, nil
}

func (c *Client) Volume(ctx context.Context, name string) (*dto.VolumeResponse, error) {
	if err := c.requireProject(); err != nil {
		return nil, err
	}
	path := c.projectPath() + "/volumes/" + url.PathEscape(name)
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("volume request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var out dto.VolumeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding volume: %w", err)
	}
	return &out, nil
}
