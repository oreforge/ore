package deploy

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oreforge/ore/internal/spec"
)

func TestSortedEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]string
		want []string
	}{
		{name: "nil_map", m: nil, want: []string{}},
		{name: "empty_map", m: map[string]string{}, want: []string{}},
		{
			name: "sorted_output",
			m:    map[string]string{"B": "2", "A": "1", "C": "3"},
			want: []string{"A=1", "B=2", "C=3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sortedEnv(tt.m)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildEnvList(t *testing.T) {
	t.Parallel()

	t.Run("with_memory", func(t *testing.T) {
		t.Parallel()
		srv := &spec.Server{
			Memory: "2G",
			Env:    map[string]string{"EULA": "true"},
		}
		env := buildEnvList(srv)
		assert.Contains(t, env, "ORE_MEMORY=2G")
		assert.Contains(t, env, "EULA=true")
		assert.Equal(t, "ORE_MEMORY=2G", env[0])
	})

	t.Run("without_memory", func(t *testing.T) {
		t.Parallel()
		srv := &spec.Server{
			Env: map[string]string{"A": "1", "B": "2"},
		}
		env := buildEnvList(srv)
		assert.Equal(t, []string{"A=1", "B=2"}, env)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		srv := &spec.Server{}
		env := buildEnvList(srv)
		assert.Empty(t, env)
	})
}

func TestResolveServiceHealthCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hc   *spec.HealthCheck
		want *spec.HealthCheck
	}{
		{name: "nil", hc: nil, want: nil},
		{name: "disabled", hc: &spec.HealthCheck{Disabled: true}, want: nil},
		{name: "empty_cmd", hc: &spec.HealthCheck{Cmd: ""}, want: nil},
		{
			name: "defaults_applied",
			hc:   &spec.HealthCheck{Cmd: "pg_isready"},
			want: &spec.HealthCheck{
				Cmd:         "pg_isready",
				Interval:    10_000_000_000,
				Timeout:     5_000_000_000,
				StartPeriod: 30_000_000_000,
				Retries:     3,
			},
		},
		{
			name: "custom_values_preserved",
			hc: &spec.HealthCheck{
				Cmd:         "check",
				Interval:    1_000_000_000,
				Timeout:     1_000_000_000,
				StartPeriod: 5_000_000_000,
				Retries:     10,
			},
			want: &spec.HealthCheck{
				Cmd:         "check",
				Interval:    1_000_000_000,
				Timeout:     1_000_000_000,
				StartPeriod: 5_000_000_000,
				Retries:     10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveServiceHealthCheck(tt.hc)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBindPorts(t *testing.T) {
	t.Parallel()

	t.Run("single_port", func(t *testing.T) {
		t.Parallel()
		cc := &container.Config{}
		hc := &container.HostConfig{}
		require.NoError(t, bindPorts([]string{"25565"}, cc, hc))
		assert.Len(t, cc.ExposedPorts, 1)
		_, ok := cc.ExposedPorts[nat.Port("25565/tcp")]
		assert.True(t, ok)
		assert.Len(t, hc.PortBindings[nat.Port("25565/tcp")], 1)
		assert.Equal(t, "25565", hc.PortBindings[nat.Port("25565/tcp")][0].HostPort)
	})

	t.Run("host_container_mapping", func(t *testing.T) {
		t.Parallel()
		cc := &container.Config{}
		hc := &container.HostConfig{}
		require.NoError(t, bindPorts([]string{"8080:25565"}, cc, hc))
		_, ok := cc.ExposedPorts[nat.Port("25565/tcp")]
		assert.True(t, ok)
		assert.Equal(t, "8080", hc.PortBindings[nat.Port("25565/tcp")][0].HostPort)
	})

	t.Run("multiple_ports", func(t *testing.T) {
		t.Parallel()
		cc := &container.Config{}
		hc := &container.HostConfig{}
		require.NoError(t, bindPorts([]string{"25565", "8080:25566"}, cc, hc))
		assert.Len(t, cc.ExposedPorts, 2)
	})

	t.Run("invalid_port", func(t *testing.T) {
		t.Parallel()
		cc := &container.Config{}
		hc := &container.HostConfig{}
		err := bindPorts([]string{"abc"}, cc, hc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid port")
	})
}

func TestParseMemory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		{name: "gigabytes", input: "2G", want: 2 * 1024 * 1024 * 1024},
		{name: "megabytes", input: "512M", want: 512 * 1024 * 1024},
		{name: "fractional_gigabytes", input: "1.5G", want: int64(1.5 * 1024 * 1024 * 1024)},
		{name: "lowercase_g", input: "2g", want: 2 * 1024 * 1024 * 1024},
		{name: "lowercase_m", input: "512m", want: 512 * 1024 * 1024},
		{name: "invalid", input: "abc", wantErr: "invalid memory"},
		{name: "too_short", input: "M", wantErr: "invalid memory"},
		{name: "unknown_suffix", input: "2T", wantErr: "unknown memory suffix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMemory(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseCPU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		{name: "whole_number", input: "2", want: 2_000_000_000},
		{name: "fractional", input: "0.5", want: 500_000_000},
		{name: "invalid", input: "abc", wantErr: "invalid cpu value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCPU(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "net_container_vol", VolumeNameFor("net", "container", "vol"))
}
