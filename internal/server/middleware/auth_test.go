package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBearerAuth(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		headers    map[string]string
		query      string
		wantStatus int
	}{
		{
			name:       "valid_bearer",
			headers:    map[string]string{"Authorization": "Bearer my-secret-token"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_authorization",
			headers:    map[string]string{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong_token",
			headers:    map[string]string{"Authorization": "Bearer wrong-token"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer_prefix_only",
			headers:    map[string]string{"Authorization": "Bearer "},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "websocket_with_query_token",
			headers:    map[string]string{"Upgrade": "websocket"},
			query:      "token=my-secret-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "websocket_without_token",
			headers:    map[string]string{"Upgrade": "websocket"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "websocket_wrong_token",
			headers:    map[string]string{"Upgrade": "websocket"},
			query:      "token=wrong",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mw := BearerAuth("my-secret-token")
			wrapped := mw(handler)

			url := "/"
			if tt.query != "" {
				url = "/?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		upgrade string
		want    bool
	}{
		{name: "lowercase", upgrade: "websocket", want: true},
		{name: "mixed_case", upgrade: "WebSocket", want: true},
		{name: "empty", upgrade: "", want: false},
		{name: "other", upgrade: "h2c", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}
			assert.Equal(t, tt.want, isWebSocketUpgrade(req))
		})
	}
}
