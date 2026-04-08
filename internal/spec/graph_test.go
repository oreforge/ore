package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNoCycles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		network Network
		wantErr string
	}{
		{
			name: "no_dependencies",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21"},
					{Name: "b", Dir: "./b", Software: "paper:1.21"},
				},
			},
		},
		{
			name: "linear_chain",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: ConditionStarted}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21", DependsOn: []Dependency{{Name: "c", Condition: ConditionStarted}}},
					{Name: "c", Dir: "./c", Software: "paper:1.21"},
				},
			},
		},
		{
			name: "simple_cycle",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: ConditionStarted}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21", DependsOn: []Dependency{{Name: "a", Condition: ConditionStarted}}},
				},
			},
			wantErr: "circular dependency",
		},
		{
			name: "three_node_cycle",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: ConditionStarted}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21", DependsOn: []Dependency{{Name: "c", Condition: ConditionStarted}}},
					{Name: "c", Dir: "./c", Software: "paper:1.21", DependsOn: []Dependency{{Name: "a", Condition: ConditionStarted}}},
				},
			},
			wantErr: "circular dependency",
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

func TestTopologicalOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		net    *Network
		groups []StartGroup
	}{
		{
			name: "single_server_no_deps",
			net: &Network{
				Servers: []Server{{Name: "a"}},
			},
			groups: []StartGroup{
				{Servers: []string{"a"}},
			},
		},
		{
			name: "two_independent_servers",
			net: &Network{
				Servers: []Server{{Name: "a"}, {Name: "b"}},
			},
			groups: []StartGroup{
				{Servers: []string{"a", "b"}},
			},
		},
		{
			name: "linear_chain",
			net: &Network{
				Servers: []Server{
					{Name: "a", DependsOn: []Dependency{{Name: "b"}}},
					{Name: "b", DependsOn: []Dependency{{Name: "c"}}},
					{Name: "c"},
				},
			},
			groups: []StartGroup{
				{Servers: []string{"c"}},
				{Servers: []string{"b"}},
				{Servers: []string{"a"}},
			},
		},
		{
			name: "diamond_dependency",
			net: &Network{
				Servers: []Server{
					{Name: "a", DependsOn: []Dependency{{Name: "b"}, {Name: "c"}}},
					{Name: "b", DependsOn: []Dependency{{Name: "d"}}},
					{Name: "c", DependsOn: []Dependency{{Name: "d"}}},
					{Name: "d"},
				},
			},
			groups: []StartGroup{
				{Servers: []string{"d"}},
				{Servers: []string{"b", "c"}},
				{Servers: []string{"a"}},
			},
		},
		{
			name: "mixed_servers_and_services",
			net: &Network{
				Servers: []Server{
					{Name: "lobby", DependsOn: []Dependency{{Name: "db"}}},
				},
				Services: []Service{
					{Name: "db"},
				},
			},
			groups: []StartGroup{
				{Services: []string{"db"}},
				{Servers: []string{"lobby"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := TopologicalOrder(tt.net)
			require.Len(t, got, len(tt.groups))
			for i, wantGroup := range tt.groups {
				assert.ElementsMatch(t, wantGroup.Servers, got[i].Servers, "group %d servers", i)
				assert.ElementsMatch(t, wantGroup.Services, got[i].Services, "group %d services", i)
			}
		})
	}
}

func TestResolveDependencyConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		net           *Network
		depName       string
		serverIdx     int
		wantCondition DependencyCondition
	}{
		{
			name: "target_with_healthcheck_resolves_to_healthy",
			net: &Network{
				Servers: []Server{
					{Name: "a", DependsOn: []Dependency{{Name: "b"}}},
					{Name: "b", HealthCheck: &HealthCheck{Cmd: "check"}},
				},
			},
			serverIdx:     0,
			wantCondition: ConditionHealthy,
		},
		{
			name: "target_without_healthcheck_resolves_to_started",
			net: &Network{
				Servers: []Server{
					{Name: "a", DependsOn: []Dependency{{Name: "b"}}},
					{Name: "b"},
				},
			},
			serverIdx:     0,
			wantCondition: ConditionStarted,
		},
		{
			name: "target_with_disabled_healthcheck_resolves_to_started",
			net: &Network{
				Servers: []Server{
					{Name: "a", DependsOn: []Dependency{{Name: "b"}}},
					{Name: "b", HealthCheck: &HealthCheck{Disabled: true}},
				},
			},
			serverIdx:     0,
			wantCondition: ConditionStarted,
		},
		{
			name: "explicit_condition_unchanged",
			net: &Network{
				Servers: []Server{
					{Name: "a", DependsOn: []Dependency{{Name: "b", Condition: ConditionStarted}}},
					{Name: "b", HealthCheck: &HealthCheck{Cmd: "check"}},
				},
			},
			serverIdx:     0,
			wantCondition: ConditionStarted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ResolveDependencyConditions(tt.net)
			assert.Equal(t, tt.wantCondition, tt.net.Servers[tt.serverIdx].DependsOn[0].Condition)
		})
	}
}

func TestValidateDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		network Network
		wantErr string
	}{
		{
			name: "self_reference",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "a", Condition: ConditionStarted}}},
				},
			},
			wantErr: "cannot reference itself",
		},
		{
			name: "unknown_dependency",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "missing", Condition: ConditionStarted}}},
				},
			},
			wantErr: "unknown name",
		},
		{
			name: "healthy_without_healthcheck",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: ConditionHealthy}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21"},
				},
			},
			wantErr: "has no healthcheck",
		},
		{
			name: "healthy_with_disabled_healthcheck",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: ConditionHealthy}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21", HealthCheck: &HealthCheck{Disabled: true}},
				},
			},
			wantErr: "has no healthcheck",
		},
		{
			name: "invalid_condition",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: "bogus"}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21"},
				},
			},
			wantErr: "invalid condition",
		},
		{
			name: "healthy_with_healthcheck_passes",
			network: Network{
				Network: "test",
				Servers: []Server{
					{Name: "a", Dir: "./a", Software: "paper:1.21", DependsOn: []Dependency{{Name: "b", Condition: ConditionHealthy}}},
					{Name: "b", Dir: "./b", Software: "paper:1.21", HealthCheck: &HealthCheck{Cmd: "check"}},
				},
			},
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
