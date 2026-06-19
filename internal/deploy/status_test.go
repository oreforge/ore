package deploy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/spec"
)

func TestWorkloadSpecForServer(t *testing.T) {
	t.Parallel()

	t.Run("minimal", func(t *testing.T) {
		t.Parallel()
		srv := &spec.Server{Name: "lobby", Software: "paper:1.20.6"}
		ws := workloadSpecForServer(srv, nil)

		assert.Equal(t, "software", ws.Source.Type)
		require.NotNil(t, ws.Source.Software)
		assert.Equal(t, "paper:1.20.6", ws.Source.Software.Raw)
		assert.Equal(t, "paper", ws.Source.Software.Name)
		assert.Equal(t, "1.20.6", ws.Source.Software.Version)
		assert.Empty(t, ws.Source.Image)
		assert.Empty(t, ws.Ports)
		assert.Nil(t, ws.HealthCheck)
	})

	t.Run("full", func(t *testing.T) {
		t.Parallel()
		hc := &spec.HealthCheck{Cmd: "echo ok", Interval: 5 * time.Second}
		srv := &spec.Server{
			Name:        "survival",
			Software:    "folia:1.21.0",
			Ports:       []string{"25565:25565", "19132:19132/udp"},
			Memory:      "4Gi",
			CPU:         "2",
			Env:         map[string]string{"FOO": "bar"},
			Volumes:     []spec.Volume{{Name: "world", Target: "/data/world"}},
			DependsOn:   []spec.Dependency{{Name: "proxy", Condition: spec.ConditionHealthy}},
			HealthCheck: hc,
		}
		ws := workloadSpecForServer(srv, nil)

		assert.Equal(t, "folia", ws.Source.Software.Name)
		assert.Equal(t, "1.21.0", ws.Source.Software.Version)
		assert.Len(t, ws.Ports, 1)
		assert.Equal(t, 25565, ws.Ports[0].Host)
		assert.Equal(t, "4Gi", ws.Memory)
		assert.Equal(t, "2", ws.CPU)
		assert.Equal(t, map[string]string{"FOO": "bar"}, ws.Env)
		assert.Len(t, ws.Volumes, 1)
		assert.Len(t, ws.DependsOn, 1)
		assert.Same(t, hc, ws.HealthCheck)
	})

	t.Run("malformed_software", func(t *testing.T) {
		t.Parallel()
		srv := &spec.Server{Name: "broken", Software: "no-version"}
		ws := workloadSpecForServer(srv, nil)

		assert.Equal(t, "software", ws.Source.Type)
		require.NotNil(t, ws.Source.Software)
		assert.Equal(t, "no-version", ws.Source.Software.Raw)
		assert.Empty(t, ws.Source.Software.Name)
		assert.Empty(t, ws.Source.Software.Version)
	})

	t.Run("disabled_healthcheck_passes_through", func(t *testing.T) {
		t.Parallel()
		hc := &spec.HealthCheck{Disabled: true}
		srv := &spec.Server{Name: "n", Software: "paper:1.20.6", HealthCheck: hc}
		ws := workloadSpecForServer(srv, nil)
		require.NotNil(t, ws.HealthCheck)
		assert.True(t, ws.HealthCheck.Disabled)
	})

	t.Run("unparseable_port_skipped", func(t *testing.T) {
		t.Parallel()
		srv := &spec.Server{
			Name:     "n",
			Software: "paper:1.20.6",
			Ports:    []string{"25565:25565", "garbage"},
		}
		ws := workloadSpecForServer(srv, nil)
		assert.Len(t, ws.Ports, 1)
	})
}

func TestWorkloadSpecForService(t *testing.T) {
	t.Parallel()

	svc := &spec.Service{
		Name:    "redis",
		Image:   "redis:7-alpine",
		Ports:   []string{"6379:6379"},
		Env:     map[string]string{"FOO": "bar"},
		Volumes: []spec.Volume{{Name: "data", Target: "/data"}},
	}
	ws := workloadSpecForService(svc, nil)

	assert.Equal(t, "image", ws.Source.Type)
	assert.Nil(t, ws.Source.Software)
	assert.Equal(t, "redis:7-alpine", ws.Source.Image)
	assert.Len(t, ws.Ports, 1)
	assert.Equal(t, map[string]string{"FOO": "bar"}, ws.Env)
}

func TestBuildInfoFor(t *testing.T) {
	t.Parallel()

	t.Run("nil_manifest", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, buildInfoFor(nil, "lobby"))
	})

	t.Run("server_not_in_manifest", func(t *testing.T) {
		t.Parallel()
		m := build.NewManifest()
		assert.Nil(t, buildInfoFor(m, "missing"))
	})

	t.Run("returns_latest_entry", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		m := build.NewManifest()
		m.Builds["lobby-old"] = build.Entry{
			ServerName: "lobby", ImageTag: "ore/lobby:old", CacheKey: "old",
			BaseImage: "alpine:3.18", BuiltAt: now.Add(-time.Hour),
		}
		m.Builds["lobby-new"] = build.Entry{
			ServerName: "lobby", ImageTag: "ore/lobby:new", CacheKey: "new",
			BaseImage: "alpine:3.19", BuiltAt: now,
		}
		m.Builds["other-x"] = build.Entry{ServerName: "other", ImageTag: "ore/other:x", BuiltAt: now}

		bi := buildInfoFor(m, "lobby")
		require.NotNil(t, bi)
		assert.Equal(t, "ore/lobby:new", bi.ImageTag)
		assert.Equal(t, "new", bi.CacheKey)
		assert.Equal(t, "alpine:3.19", bi.BaseImage)
	})
}
