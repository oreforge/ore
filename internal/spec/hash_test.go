package spec

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServerHash(t *testing.T) {
	t.Parallel()

	hexPattern := regexp.MustCompile(`^[0-9a-f]{12}$`)

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		srv := &Server{Ports: []string{"25565"}, Memory: "2G"}
		h1 := ServerHash(srv, "ore/lobby:abc123")
		h2 := ServerHash(srv, "ore/lobby:abc123")
		assert.Equal(t, h1, h2)
		assert.Regexp(t, hexPattern, h1)
	})

	t.Run("different_image_tag", func(t *testing.T) {
		t.Parallel()
		srv := &Server{Ports: []string{"25565"}}
		h1 := ServerHash(srv, "ore/lobby:abc123")
		h2 := ServerHash(srv, "ore/lobby:def456")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("different_ports", func(t *testing.T) {
		t.Parallel()
		s1 := &Server{Ports: []string{"25565"}}
		s2 := &Server{Ports: []string{"25566"}}
		assert.NotEqual(t, ServerHash(s1, "tag"), ServerHash(s2, "tag"))
	})

	t.Run("different_env", func(t *testing.T) {
		t.Parallel()
		s1 := &Server{Env: map[string]string{"A": "1"}}
		s2 := &Server{Env: map[string]string{"A": "2"}}
		assert.NotEqual(t, ServerHash(s1, "tag"), ServerHash(s2, "tag"))
	})

	t.Run("env_order_irrelevant", func(t *testing.T) {
		t.Parallel()
		s1 := &Server{Env: map[string]string{"A": "1", "B": "2"}}
		s2 := &Server{Env: map[string]string{"B": "2", "A": "1"}}
		assert.Equal(t, ServerHash(s1, "tag"), ServerHash(s2, "tag"))
	})

	t.Run("different_volumes", func(t *testing.T) {
		t.Parallel()
		s1 := &Server{Volumes: []Volume{{Name: "data", Target: "/data"}}}
		s2 := &Server{Volumes: []Volume{{Name: "data", Target: "/other"}}}
		assert.NotEqual(t, ServerHash(s1, "tag"), ServerHash(s2, "tag"))
	})

	t.Run("nil_vs_disabled_healthcheck", func(t *testing.T) {
		t.Parallel()
		s1 := &Server{HealthCheck: nil}
		s2 := &Server{HealthCheck: &HealthCheck{Disabled: true}}
		assert.NotEqual(t, ServerHash(s1, "tag"), ServerHash(s2, "tag"))
	})

	t.Run("empty_server_no_panic", func(t *testing.T) {
		t.Parallel()
		h := ServerHash(&Server{}, "")
		assert.Regexp(t, hexPattern, h)
	})
}

func TestServiceHash(t *testing.T) {
	t.Parallel()

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		svc := &Service{Image: "postgres:16", Ports: []string{"5432"}}
		h1 := ServiceHash(svc)
		h2 := ServiceHash(svc)
		assert.Equal(t, h1, h2)
	})

	t.Run("different_image", func(t *testing.T) {
		t.Parallel()
		s1 := &Service{Image: "postgres:16"}
		s2 := &Service{Image: "postgres:15"}
		assert.NotEqual(t, ServiceHash(s1), ServiceHash(s2))
	})

	t.Run("with_healthcheck", func(t *testing.T) {
		t.Parallel()
		s1 := &Service{Image: "postgres:16"}
		s2 := &Service{Image: "postgres:16", HealthCheck: &HealthCheck{Cmd: "pg_isready", Interval: 5 * time.Second}}
		assert.NotEqual(t, ServiceHash(s1), ServiceHash(s2))
	})
}
