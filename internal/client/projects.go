package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

func (c *Client) AddProject(ctx context.Context, repoURL, name string) (string, error) {
	payload := map[string]string{"url": repoURL}
	if name != "" {
		payload["name"] = name
	}
	body, _ := json.Marshal(payload)
	req, err := c.newRequest(ctx, "POST", "/api/projects", body)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("add project request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 201 {
		return "", c.readError(resp)
	}

	var result struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return result.Name, nil
}

func (c *Client) RemoveProject(ctx context.Context, name string) error {
	req, err := c.newRequest(ctx, "DELETE", "/api/projects/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("remove project request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 204 {
		return c.readError(resp)
	}
	return nil
}

func (c *Client) UpdateProject(ctx context.Context, name string) (string, error) {
	req, err := c.newRequest(ctx, "PATCH", "/api/projects/"+url.PathEscape(name), nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("update project request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", c.readError(resp)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return result.Status, nil
}

func (c *Client) ListProjects(ctx context.Context) ([]string, error) {
	req, err := c.newRequest(ctx, "GET", "/api/projects", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list projects request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, c.readError(resp)
	}

	var result struct {
		Projects []string `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding projects: %w", err)
	}
	return result.Projects, nil
}
