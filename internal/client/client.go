package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/coder/websocket"

	"github.com/oreforge/ore/internal/console"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/project"
)

type Client struct {
	addr    string
	token   string
	project string
	client  *http.Client
}

func New(addr, token, project string) (*Client, error) {
	if project == "" {
		return nil, fmt.Errorf("no active project set (use 'ore projects use <name>' to select one)")
	}
	return &Client{
		addr:    addr,
		token:   token,
		project: project,
		client:  &http.Client{},
	}, nil
}

func (c *Client) projectPath() string {
	return "/api/projects/" + url.PathEscape(c.project)
}

func (c *Client) Up(ctx context.Context, opts project.UpOptions) error {
	body, _ := json.Marshal(map[string]any{"no_cache": opts.NoCache, "force": opts.Force})
	return c.streamRequest(ctx, "POST", c.projectPath()+"/up", body)
}

func (c *Client) Down(ctx context.Context) error {
	return c.streamRequest(ctx, "POST", c.projectPath()+"/down", nil)
}

func (c *Client) Build(ctx context.Context, noCache bool) error {
	body, _ := json.Marshal(map[string]any{"no_cache": noCache})
	return c.streamRequest(ctx, "POST", c.projectPath()+"/build", body)
}

func (c *Client) Status(ctx context.Context) (*deploy.NetworkStatus, error) {
	req, err := c.newRequest(ctx, "GET", c.projectPath()+"/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var status deploy.NetworkStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding status: %w", err)
	}
	return &status, nil
}

func (c *Client) Prune(ctx context.Context, target project.PruneTarget) error {
	body, _ := json.Marshal(map[string]any{"target": target.String()})
	return c.streamRequest(ctx, "POST", c.projectPath()+"/prune", body)
}

func (c *Client) Clean(ctx context.Context, target project.CleanTarget) error {
	body, _ := json.Marshal(map[string]any{"target": target.String()})
	return c.streamRequest(ctx, "POST", c.projectPath()+"/clean", body)
}

func (c *Client) Console(ctx context.Context, serverName string) error {
	u := url.URL{
		Scheme:   "ws",
		Host:     c.addr,
		Path:     c.projectPath() + "/console",
		RawQuery: "server=" + url.QueryEscape(serverName),
	}

	headers := http.Header{}
	if c.token != "" {
		headers.Set("Authorization", "Bearer "+c.token)
	}

	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return fmt.Errorf("console websocket: %w", err)
	}
	conn.SetReadLimit(128 * 1024)

	return console.Run(ctx, &console.WSConn{Conn: conn})
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.addr+path, bodyReader)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) streamRequest(ctx context.Context, method, path string, body []byte) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	return drainNDJSON(resp.Body)
}

func (c *Client) readError(resp *http.Response) error {
	var errResp struct {
		Detail string `json:"detail"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Detail != "" {
		return fmt.Errorf("ored: %s (HTTP %d)", errResp.Detail, resp.StatusCode)
	}
	return fmt.Errorf("ored: unexpected status %d", resp.StatusCode)
}
