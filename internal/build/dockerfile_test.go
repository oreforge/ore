package build

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/oreforge/ore/internal/software"
	"github.com/oreforge/ore/internal/spec"
)

func TestGenerateDockerfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     DockerfileOptions
		contains []string
		excludes []string
	}{
		{
			name: "entrypoint_mode",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
					Entrypoint: "entrypoint.sh",
				},
			},
			contains: []string{
				"FROM eclipse-temurin:21-jre-alpine",
				"COPY server.jar /opt/ore/server.jar",
				"COPY entrypoint.sh /opt/ore/entrypoint.sh",
				"ENTRYPOINT [\"tini\", \"-s\", \"--\", \"/opt/ore/entrypoint.sh\"]",
				"WORKDIR /data",
				"EXPOSE 25565",
			},
			excludes: []string{"CMD"},
		},
		{
			name: "direct_mode",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
				},
			},
			contains: []string{
				"ENTRYPOINT [\"tini\", \"-s\", \"--\", \"/opt/ore/server.jar\"]",
			},
			excludes: []string{"entrypoint.sh", "CMD"},
		},
		{
			name: "with_extra_args",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
					Entrypoint: "entrypoint.sh",
				},
				ExtraArgs: "--nogui",
			},
			contains: []string{
				"CMD [\"--nogui\"]",
			},
		},
		{
			name: "without_extra_args",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
				},
			},
			excludes: []string{"CMD"},
		},
		{
			name: "with_healthcheck",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
				},
				HealthCheck: &spec.HealthCheck{
					Cmd:         "mc-health",
					Interval:    5 * time.Second,
					Timeout:     3 * time.Second,
					StartPeriod: 10 * time.Second,
					Retries:     5,
				},
			},
			contains: []string{
				"HEALTHCHECK",
				"--interval=5s",
				"--timeout=3s",
				"--start-period=10s",
				"--retries=5",
				"mc-health",
			},
		},
		{
			name: "without_healthcheck",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
				},
			},
			excludes: []string{"HEALTHCHECK"},
		},
		{
			name: "disabled_healthcheck",
			opts: DockerfileOptions{
				Runtime: software.Runtime{
					BaseImage:  "eclipse-temurin:21-jre-alpine",
					BinaryName: "server.jar",
				},
				HealthCheck: &spec.HealthCheck{Disabled: true},
			},
			excludes: []string{"HEALTHCHECK"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateDockerfile(tt.opts)
			for _, s := range tt.contains {
				assert.Contains(t, result, s)
			}
			for _, s := range tt.excludes {
				assert.NotContains(t, result, s)
			}
		})
	}
}

func TestDockerHealthcheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hc   *spec.HealthCheck
		want string
	}{
		{name: "nil", hc: nil, want: ""},
		{name: "disabled", hc: &spec.HealthCheck{Disabled: true}, want: ""},
		{name: "empty_cmd", hc: &spec.HealthCheck{Cmd: ""}, want: ""},
		{
			name: "defaults_applied",
			hc:   &spec.HealthCheck{Cmd: "check"},
			want: "--interval=2s --timeout=2s --start-period=5s --retries=3",
		},
		{
			name: "custom_values",
			hc: &spec.HealthCheck{
				Cmd:         "pg_isready",
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				StartPeriod: 30 * time.Second,
				Retries:     5,
			},
			want: "--interval=10s --timeout=5s --start-period=30s --retries=5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := dockerHealthcheck(tt.hc)
			if tt.want == "" {
				assert.Empty(t, result)
				return
			}
			for _, part := range strings.Fields(tt.want) {
				assert.Contains(t, result, part)
			}
		})
	}
}
