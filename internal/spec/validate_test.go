package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	minimalServer := Server{
		Name:     "lobby",
		Dir:      "./lobby",
		Software: "paper:1.21",
	}

	tests := []struct {
		name    string
		network Network
		wantErr string
	}{
		{
			name:    "missing_network_name",
			network: Network{Servers: []Server{minimalServer}},
			wantErr: "network name is required",
		},
		{
			name:    "no_servers",
			network: Network{Network: "test"},
			wantErr: "at least one server is required",
		},
		{
			name: "valid_minimal",
			network: Network{
				Network: "test",
				Servers: []Server{minimalServer},
			},
		},
		{
			name: "server_missing_name",
			network: Network{
				Network: "test",
				Servers: []Server{{Dir: "./lobby", Software: "paper:1.21"}},
			},
			wantErr: "servers[0].name is required",
		},
		{
			name: "server_missing_dir",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Software: "paper:1.21"}},
			},
			wantErr: "dir is required",
		},
		{
			name: "server_missing_software",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby"}},
			},
			wantErr: "software is required",
		},
		{
			name: "duplicate_server_names",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "lobby", Dir: "./lobby", Software: "paper:1.21"},
					{Name: "lobby", Dir: "./lobby2", Software: "paper:1.21"},
				},
			},
			wantErr: "duplicate name",
		},
		{
			name: "duplicate_server_service_name",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "db", Dir: "./db", Software: "paper:1.21"},
				},
				Services: []Service{
					{Name: "db", Image: "postgres:16"},
				},
			},
			wantErr: "duplicate name",
		},
		{
			name: "invalid_software_format",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby", Software: "paper"}},
			},
			wantErr: "must be in format name:version",
		},
		{
			name: "invalid_port",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby", Software: "paper:1.21", Ports: []string{"abc"}}},
			},
			wantErr: "invalid port",
		},
		{
			name: "port_out_of_range_zero",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby", Software: "paper:1.21", Ports: []string{"0"}}},
			},
			wantErr: "out of range",
		},
		{
			name: "port_out_of_range_high",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby", Software: "paper:1.21", Ports: []string{"99999"}}},
			},
			wantErr: "out of range",
		},
		{
			name: "volume_missing_name",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby", Software: "paper:1.21", Volumes: []Volume{{Target: "/data"}}}},
			},
			wantErr: "volumes[0].name is required",
		},
		{
			name: "volume_missing_target",
			network: Network{
				Network: "test",
				Servers: []Server{{Name: "lobby", Dir: "./lobby", Software: "paper:1.21", Volumes: []Volume{{Name: "data"}}}},
			},
			wantErr: "volumes[0].target is required",
		},
		{
			name: "service_missing_name",
			network: Network{
				Network: "test",
				Servers: []Server{minimalServer},
				Services: []Service{
					{Image: "postgres:16"},
				},
			},
			wantErr: "services[0].name is required",
		},
		{
			name: "service_missing_image",
			network: Network{
				Network: "test",
				Servers: []Server{minimalServer},
				Services: []Service{
					{Name: "db"},
				},
			},
			wantErr: "image is required",
		},
		{
			name: "service_invalid_image_format",
			network: Network{
				Network: "test",
				Servers: []Server{minimalServer},
				Services: []Service{
					{Name: "db", Image: "postgres"},
				},
			},
			wantErr: "must be in format name:tag",
		},
		{
			name: "healthcheck_negative_interval",
			network: Network{
				Network: "test",
				Servers: []Server{{
					Name: "lobby", Dir: "./lobby", Software: "paper:1.21",
					HealthCheck: &HealthCheck{Interval: -1},
				}},
			},
			wantErr: "healthcheck.interval must be positive",
		},
		{
			name: "healthcheck_negative_retries",
			network: Network{
				Network: "test",
				Servers: []Server{{
					Name: "lobby", Dir: "./lobby", Software: "paper:1.21",
					HealthCheck: &HealthCheck{Retries: -1},
				}},
			},
			wantErr: "healthcheck.retries must be positive",
		},
		{
			name: "healthcheck_disabled_skips_validation",
			network: Network{
				Network: "test",
				Servers: []Server{{
					Name: "lobby", Dir: "./lobby", Software: "paper:1.21",
					HealthCheck: &HealthCheck{Disabled: true, Interval: -1, Retries: -1},
				}},
			},
		},
		{
			name: "healthcheck_nil_passes",
			network: Network{
				Network: "test",
				Servers: []Server{{
					Name: "lobby", Dir: "./lobby", Software: "paper:1.21",
					HealthCheck: nil,
				}},
			},
		},
		{
			name: "valid_full_spec",
			network: Network{
				Network: "test",
				Servers: []Server{
					{
						Name: "lobby", Dir: "./lobby", Software: "paper:1.21",
						Ports:       []string{"25565"},
						Memory:      "2G",
						Env:         map[string]string{"EULA": "true"},
						Volumes:     []Volume{{Name: "world", Target: "/data/world"}},
						HealthCheck: &HealthCheck{Cmd: "mc-health"},
						DependsOn:   []Dependency{{Name: "db", Condition: ConditionHealthy}},
					},
				},
				Services: []Service{
					{
						Name:        "db",
						Image:       "postgres:16",
						Ports:       []string{"5432"},
						HealthCheck: &HealthCheck{Cmd: "pg_isready"},
					},
				},
			},
		},
		{
			name: "service_volume_missing_name",
			network: Network{
				Network: "test",
				Servers: []Server{minimalServer},
				Services: []Service{
					{Name: "db", Image: "postgres:16", Volumes: []Volume{{Target: "/data"}}},
				},
			},
			wantErr: "volumes[0].name is required",
		},
		{
			name: "service_healthcheck_negative_timeout",
			network: Network{
				Network: "test",
				Servers: []Server{minimalServer},
				Services: []Service{
					{Name: "db", Image: "postgres:16", HealthCheck: &HealthCheck{Timeout: -1}},
				},
			},
			wantErr: "healthcheck.timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(&tt.network)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    PortMapping
		wantErr string
	}{
		{
			name:  "single_port",
			input: "25565",
			want:  PortMapping{Host: 25565, Container: 25565},
		},
		{
			name:  "host_container_mapping",
			input: "8080:25565",
			want:  PortMapping{Host: 8080, Container: 25565},
		},
		{
			name:    "non_numeric",
			input:   "abc",
			wantErr: "invalid port",
		},
		{
			name:    "zero",
			input:   "0",
			wantErr: "out of range",
		},
		{
			name:    "exceeds_max",
			input:   "65536",
			wantErr: "out of range",
		},
		{
			name:    "invalid_container_port",
			input:   "8080:abc",
			wantErr: "invalid port",
		},
		{
			name:  "whitespace_trimmed",
			input: " 8080 : 25565 ",
			want:  PortMapping{Host: 8080, Container: 25565},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePort(tt.input)
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

func TestValidateSoftwareFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: "paper:1.21"},
		{name: "missing_version", input: "paper", wantErr: "must be in format name:version"},
		{name: "empty_name", input: ":1.21", wantErr: "must be in format name:version"},
		{name: "empty_version", input: "paper:", wantErr: "must be in format name:version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSoftwareFormat(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateImageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: "postgres:16"},
		{name: "missing_tag", input: "postgres", wantErr: "must be in format name:tag"},
		{name: "empty_name", input: ":16", wantErr: "must be in format name:tag"},
		{name: "empty_tag", input: "postgres:", wantErr: "must be in format name:tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateImageFormat(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
