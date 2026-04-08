package spec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHealthCheckUnmarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("disable_string", func(t *testing.T) {
		t.Parallel()
		var s struct {
			HC HealthCheck `yaml:"hc"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(`hc: disable`), &s))
		assert.True(t, s.HC.Disabled)
	})

	t.Run("command_string", func(t *testing.T) {
		t.Parallel()
		var s struct {
			HC HealthCheck `yaml:"hc"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(`hc: "mc-health"`), &s))
		assert.False(t, s.HC.Disabled)
		assert.Equal(t, "mc-health", s.HC.Cmd)
	})

	t.Run("full_mapping", func(t *testing.T) {
		t.Parallel()
		var s struct {
			HC HealthCheck `yaml:"hc"`
		}
		input := `
hc:
  cmd: pg_isready
  interval: 5s
  timeout: 3s
  startPeriod: 10s
  retries: 5`
		require.NoError(t, yaml.Unmarshal([]byte(input), &s))
		assert.Equal(t, "pg_isready", s.HC.Cmd)
		assert.Equal(t, 5*time.Second, s.HC.Interval)
		assert.Equal(t, 3*time.Second, s.HC.Timeout)
		assert.Equal(t, 10*time.Second, s.HC.StartPeriod)
		assert.Equal(t, 5, s.HC.Retries)
	})

	t.Run("partial_mapping", func(t *testing.T) {
		t.Parallel()
		var s struct {
			HC HealthCheck `yaml:"hc"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(`hc: {cmd: check}`), &s))
		assert.Equal(t, "check", s.HC.Cmd)
		assert.Zero(t, s.HC.Interval)
		assert.Zero(t, s.HC.Retries)
	})

	t.Run("invalid_type_sequence", func(t *testing.T) {
		t.Parallel()
		var s struct {
			HC HealthCheck `yaml:"hc"`
		}
		err := yaml.Unmarshal([]byte(`hc: [1, 2]`), &s)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a string or object")
	})
}

func TestWaitTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hc   *HealthCheck
		want time.Duration
	}{
		{
			name: "nil",
			hc:   nil,
			want: 0,
		},
		{
			name: "disabled",
			hc:   &HealthCheck{Disabled: true},
			want: 0,
		},
		{
			name: "all_defaults",
			hc:   &HealthCheck{Cmd: "check"},
			want: 3 * (2*time.Second + 2*time.Second),
		},
		{
			name: "custom_values",
			hc: &HealthCheck{
				Cmd:         "check",
				Interval:    5 * time.Second,
				Timeout:     3 * time.Second,
				StartPeriod: 10 * time.Second,
				Retries:     5,
			},
			want: 10*time.Second + 5*(5*time.Second+3*time.Second),
		},
		{
			name: "partial_defaults",
			hc: &HealthCheck{
				Cmd:      "check",
				Interval: 10 * time.Second,
				Retries:  2,
			},
			want: 2 * (10*time.Second + 2*time.Second),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.hc.WaitTimeout())
		})
	}
}
