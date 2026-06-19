package client

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func TestDrainNDJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "lines_then_done",
			body: strings.Join([]string{
				`{"level":"INFO","msg":"step one"}`,
				`{"level":"INFO","msg":"step two"}`,
				`{"done":true}`,
			}, "\n") + "\n",
		},
		{
			name: "done_with_error",
			body: strings.Join([]string{
				`{"level":"INFO","msg":"working"}`,
				`{"done":true,"error":"build failed"}`,
			}, "\n") + "\n",
			wantErr: "build failed",
		},
		{
			name:    "no_completion_signal",
			body:    `{"level":"INFO","msg":"orphan"}` + "\n",
			wantErr: "stream ended without completion signal",
		},
		{
			name: "skips_malformed_lines",
			body: strings.Join([]string{
				`not json`,
				`{"level":"INFO","msg":"valid"}`,
				`{"done":true}`,
			}, "\n") + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := drainNDJSON(strings.NewReader(tt.body))
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestStreamRequestDirectNDJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/projects/demo/down", r.URL.Path)
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"level":"INFO","msg":"stopping"}` + "\n" + `{"done":true}` + "\n"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	err := c.Down(context.Background())
	require.NoError(t, err)
}

func TestStreamRequestOperationLogs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/projects/demo/build":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"op-123"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/operations/op-123/logs":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(strings.Join([]string{
				`{"level":"INFO","msg":"building"}`,
				`{"level":"INFO","msg":"built"}`,
				`{"done":true}`,
			}, "\n") + "\n"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	err := c.Build(context.Background(), false)
	require.NoError(t, err)
}

func TestStreamRequestServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(t, w, http.StatusConflict, "operation already running")
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	err := c.Down(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation already running")
	assert.Contains(t, err.Error(), "409")
}

func TestStreamRequestDoneError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"done":true,"error":"deploy aborted"}` + "\n"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "demo")

	err := c.Down(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deploy aborted")
}
