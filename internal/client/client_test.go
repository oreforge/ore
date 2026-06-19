package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oreforge/ore/internal/server/dto"
)

func writeProblem(t *testing.T, w http.ResponseWriter, status int, detail string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	body, err := json.Marshal(map[string]any{
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
	require.NoError(t, err)
	_, err = w.Write(body)
	require.NoError(t, err)
}

func newTestClient(t *testing.T, srv *httptest.Server, project string) *Client {
	t.Helper()
	c, err := New(srv.URL, "secret-token", project)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestListProjects(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/projects", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":["alpha","beta","gamma"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")

	got, err := c.ListProjects(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, got)
}

func TestStatusRoundTrip(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/projects/demo/status", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"network":"demo","servers":[{"name":"lobby"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	got, err := c.Status(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "demo", got.Network)
	require.Len(t, got.Servers, 1)
	assert.Equal(t, "lobby", got.Servers[0].Name)
}

func TestVolumesRoundTrip(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/projects/demo/volumes", r.URL.Path)
		resp := dto.VolumeListResponse{Volumes: []dto.VolumeResponse{
			{Name: "data", Project: "demo", Driver: "local", SizeBytes: 1024, InUseBy: []string{"lobby"}},
		}}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	got, err := c.Volumes(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "data", got[0].Name)
	assert.Equal(t, int64(1024), got[0].SizeBytes)
	assert.Equal(t, []string{"lobby"}, got[0].InUseBy)
}

func TestAuthorizationHeader(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")

	_, err := c.ListProjects(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", gotAuth)
}

func TestNoAuthorizationHeaderWithoutToken(t *testing.T) {
	t.Parallel()

	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasAuth = r.Header["Authorization"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[]}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.ListProjects(context.Background())
	require.NoError(t, err)
	assert.False(t, hasAuth)
}

func TestErrorDecoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		detail     string
		wantSubstr []string
	}{
		{
			name:       "not_found_with_detail",
			status:     http.StatusNotFound,
			detail:     "project not found",
			wantSubstr: []string{"project not found", "404"},
		},
		{
			name:       "internal_with_detail",
			status:     http.StatusInternalServerError,
			detail:     "boom",
			wantSubstr: []string{"boom", "500"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeProblem(t, w, tt.status, tt.detail)
			}))
			defer srv.Close()

			c := newTestClient(t, srv, "demo")

			_, err := c.Status(context.Background())
			require.Error(t, err)
			for _, sub := range tt.wantSubstr {
				assert.Contains(t, err.Error(), sub)
			}
		})
	}
}

func TestErrorDecodingNoDetail(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	_, err := c.Status(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestClose(t *testing.T) {
	t.Parallel()

	c, err := New("http://127.0.0.1:0", "", "")
	require.NoError(t, err)

	assert.NoError(t, c.Close())
	assert.NoError(t, c.Close())
}

func TestProjectAccessors(t *testing.T) {
	t.Parallel()

	c, err := New("http://127.0.0.1:0", "", "initial")
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	assert.Equal(t, "initial", c.Project())
	c.SetProject("updated")
	assert.Equal(t, "updated", c.Project())
}

func TestRequireProject(t *testing.T) {
	t.Parallel()

	c, err := New("http://127.0.0.1:0", "", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.Status(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active project set")
}

func TestParseAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		addr           string
		wantHost       string
		wantHTTPScheme string
		wantWSScheme   string
	}{
		{"https", "https://example.com:9000", "example.com:9000", "https", "wss"},
		{"http", "http://example.com:9000", "example.com:9000", "http", "ws"},
		{"bare", "example.com:9000", "example.com:9000", "http", "ws"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, httpScheme, wsScheme := parseAddr(tt.addr)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantHTTPScheme, httpScheme)
			assert.Equal(t, tt.wantWSScheme, wsScheme)
		})
	}
}
