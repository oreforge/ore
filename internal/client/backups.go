package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/oreforge/ore/internal/server/dto"
)

func (c *Client) Backups(ctx context.Context, volume string) ([]dto.BackupResponse, error) {
	if err := c.requireProject(); err != nil {
		return nil, err
	}
	path := c.projectPath() + "/backups"
	if volume != "" {
		path += "?volume=" + url.QueryEscape(volume)
	}
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backups request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var out dto.BackupListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding backups: %w", err)
	}
	return out.Backups, nil
}

func (c *Client) Backup(ctx context.Context, id string) (*dto.BackupResponse, error) {
	if err := c.requireProject(); err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, c.projectPath()+"/backups/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backup request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var out dto.BackupResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding backup: %w", err)
	}
	return &out, nil
}

func (c *Client) BackupCreate(ctx context.Context, volume string, tags []string) error {
	if err := c.requireProject(); err != nil {
		return err
	}
	body, err := json.Marshal(dto.CreateBackupRequest{Volume: volume, Tags: tags})
	if err != nil {
		return fmt.Errorf("encoding create backup request: %w", err)
	}
	return c.streamRequest(ctx, http.MethodPost, c.projectPath()+"/backups", body)
}

func (c *Client) BackupRemove(ctx context.Context, id string) error {
	if err := c.requireProject(); err != nil {
		return err
	}
	req, err := c.newRequest(ctx, http.MethodDelete, c.projectPath()+"/backups/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("backup remove: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return c.readError(resp)
	}
	return nil
}

func (c *Client) BackupRestore(ctx context.Context, id string, keepPreRestore bool) error {
	if err := c.requireProject(); err != nil {
		return err
	}
	path := c.projectPath() + "/backups/" + url.PathEscape(id) + "/restore"
	if keepPreRestore {
		path += "?keep_pre_restore=true"
	}
	return c.streamRequest(ctx, http.MethodPost, path, nil)
}

func (c *Client) BackupVerify(ctx context.Context, id string) error {
	if err := c.requireProject(); err != nil {
		return err
	}
	return c.streamRequest(ctx, http.MethodPost, c.projectPath()+"/backups/"+url.PathEscape(id)+"/verify", nil)
}
