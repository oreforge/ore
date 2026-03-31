package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/coder/websocket"

	"github.com/oreforge/ore/internal/orchestrator"
)

type Remote struct {
	addr    string
	token   string
	project string
	client  *http.Client
}

func NewRemote(addr, token, project string) (*Remote, error) {
	if project == "" {
		return nil, fmt.Errorf("no active project set (use 'ore projects use <name>' to select one)")
	}
	return &Remote{
		addr:    addr,
		token:   token,
		project: project,
		client:  &http.Client{},
	}, nil
}

func (r *Remote) Up(ctx context.Context, opts UpOptions) error {
	body, _ := json.Marshal(map[string]any{"no_cache": opts.NoCache, "force": opts.Force})
	return r.streamRequest(ctx, "POST", "/api/up", body)
}

func (r *Remote) Down(ctx context.Context) error {
	return r.streamRequest(ctx, "POST", "/api/down", nil)
}

func (r *Remote) Build(ctx context.Context, noCache bool) error {
	body, _ := json.Marshal(map[string]any{"no_cache": noCache})
	return r.streamRequest(ctx, "POST", "/api/build", body)
}

func (r *Remote) Status(ctx context.Context) (*orchestrator.NetworkStatus, error) {
	req, err := r.newRequest(ctx, "GET", "/api/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, r.readError(resp)
	}

	var status orchestrator.NetworkStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding status: %w", err)
	}
	return &status, nil
}

func (r *Remote) Prune(ctx context.Context, target PruneTarget) error {
	t, err := pruneTargetString(target)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"target": t})
	return r.streamRequest(ctx, "POST", "/api/prune", body)
}

func (r *Remote) Clean(ctx context.Context, target CleanTarget) error {
	t, err := cleanTargetString(target)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"target": t})
	return r.streamRequest(ctx, "POST", "/api/clean", body)
}

func (r *Remote) Console(ctx context.Context, serverName string) error {
	u := url.URL{
		Scheme:   "ws",
		Host:     r.addr,
		Path:     "/api/console",
		RawQuery: "server=" + url.QueryEscape(serverName),
	}

	headers := http.Header{}
	if r.token != "" {
		headers.Set("Authorization", "Bearer "+r.token)
	}
	headers.Set("X-Ore-Project", r.project)

	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return fmt.Errorf("console websocket: %w", err)
	}
	conn.SetReadLimit(128 * 1024)

	return runConsole(ctx, &wsConn{conn: conn})
}

func (r *Remote) Close() error {
	return nil
}

func (r *Remote) AddProject(ctx context.Context, repoURL, name string) (string, error) {
	body, _ := json.Marshal(map[string]string{"url": repoURL, "name": name})
	req, err := r.newRequest(ctx, "POST", "/api/projects", body)
	if err != nil {
		return "", err
	}
	req.Header.Del("X-Ore-Project")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("add project request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", r.readError(resp)
	}

	var result struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return result.Name, nil
}

func (r *Remote) RemoveProject(ctx context.Context, name string) error {
	req, err := r.newRequest(ctx, "DELETE", "/api/projects/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	req.Header.Del("X-Ore-Project")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("remove project request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return r.readError(resp)
	}
	return nil
}

func (r *Remote) UpdateProject(ctx context.Context, name string) error {
	req, err := r.newRequest(ctx, "PATCH", "/api/projects/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	req.Header.Del("X-Ore-Project")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("update project request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return r.readError(resp)
	}
	return nil
}

func (r *Remote) ListProjects(ctx context.Context) ([]string, error) {
	req, err := r.newRequest(ctx, "GET", "/api/projects", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Del("X-Ore-Project")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list projects request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, r.readError(resp)
	}

	var result struct {
		Projects []string `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding projects: %w", err)
	}
	return result.Projects, nil
}

func (r *Remote) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://"+r.addr+path, bodyReader)
	if err != nil {
		return nil, err
	}

	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	if r.project != "" {
		req.Header.Set("X-Ore-Project", r.project)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (r *Remote) streamRequest(ctx context.Context, method, path string, body []byte) error {
	req, err := r.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return r.readError(resp)
	}

	return drainNDJSON(resp.Body)
}

func (r *Remote) readError(resp *http.Response) error {
	var errResp struct {
		Detail string `json:"detail"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Detail != "" {
		return fmt.Errorf("ored: %s (HTTP %d)", errResp.Detail, resp.StatusCode)
	}
	return fmt.Errorf("ored: unexpected status %d", resp.StatusCode)
}

func drainNDJSON(body io.Reader) error {
	logger := slog.Default()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		if done, _ := line["done"].(bool); done {
			if errMsg, ok := line["error"].(string); ok {
				return fmt.Errorf("%s", errMsg)
			}
			return nil
		}

		msg, _ := line["msg"].(string)
		level, _ := line["level"].(string)

		var attrs []any
		for k, v := range line {
			if k == "time" || k == "level" || k == "msg" {
				continue
			}
			attrs = append(attrs, k, v)
		}

		logger.Log(context.Background(), parseLogLevel(level), msg, attrs...)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("ored: stream ended without completion signal")
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func pruneTargetString(t PruneTarget) (string, error) {
	switch t {
	case PruneAll:
		return "all", nil
	case PruneContainers:
		return "containers", nil
	case PruneImages:
		return "images", nil
	case PruneVolumes:
		return "volumes", nil
	default:
		return "", fmt.Errorf("unknown prune target: %d", t)
	}
}

func cleanTargetString(t CleanTarget) (string, error) {
	switch t {
	case CleanAll:
		return "all", nil
	case CleanCache:
		return "cache", nil
	case CleanBuilds:
		return "builds", nil
	default:
		return "", fmt.Errorf("unknown clean target: %d", t)
	}
}
