package spec

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type Network struct {
	Network  string    `yaml:"network"`
	Icon     string    `yaml:"icon,omitempty"`
	GitOps   *GitOps   `yaml:"gitops,omitempty"`
	Servers  []Server  `yaml:"servers"`
	Services []Service `yaml:"services,omitempty"`
}

type GitOps struct {
	Poll    GitOpsPoll    `yaml:"poll,omitempty"`
	Webhook GitOpsWebhook `yaml:"webhook,omitempty"`
}

type GitOpsPoll struct {
	Enabled  bool          `yaml:"enabled,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty"`
}

type GitOpsWebhook struct {
	Enabled bool `yaml:"enabled,omitempty"`
	Force   bool `yaml:"force,omitempty"`
	NoCache bool `yaml:"noCache,omitempty"`
}

type Server struct {
	Name        string            `yaml:"name"`
	Dir         string            `yaml:"dir"`
	Software    string            `yaml:"software"`
	Ports       []string          `yaml:"ports,omitempty"`
	Memory      string            `yaml:"memory,omitempty"`
	CPU         string            `yaml:"cpu,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Volumes     []Volume          `yaml:"volumes,omitempty"`
	HealthCheck *HealthCheck      `yaml:"healthcheck,omitempty"`
	DependsOn   []Dependency      `yaml:"depends_on,omitempty"`
}

type Service struct {
	Name        string            `yaml:"name"`
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Volumes     []Volume          `yaml:"volumes,omitempty"`
	HealthCheck *HealthCheck      `yaml:"healthcheck,omitempty"`
	DependsOn   []Dependency      `yaml:"depends_on,omitempty"`
}

type DependencyCondition string

const (
	ConditionStarted DependencyCondition = "started"
	ConditionHealthy DependencyCondition = "healthy"
)

type Dependency struct {
	Name      string              `yaml:"name"`
	Condition DependencyCondition `yaml:"condition,omitempty"`
}

type HealthCheck struct {
	Disabled    bool          `yaml:"-"`
	Cmd         string        `yaml:"cmd,omitempty"`
	Interval    time.Duration `yaml:"interval,omitempty"`
	Timeout     time.Duration `yaml:"timeout,omitempty"`
	StartPeriod time.Duration `yaml:"startPeriod,omitempty"`
	Retries     int           `yaml:"retries,omitempty"`
}

func (hc *HealthCheck) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		s := value.Value
		if s == "disable" {
			hc.Disabled = true
			return nil
		}
		hc.Cmd = s
		return nil
	}

	if value.Kind == yaml.MappingNode {
		type raw HealthCheck
		var r raw
		if err := value.Decode(&r); err != nil {
			return fmt.Errorf("decoding healthcheck: %w", err)
		}
		*hc = HealthCheck(r)
		return nil
	}

	return fmt.Errorf("healthcheck must be a string or object, got %v", value.Kind)
}

func (hc *HealthCheck) WaitTimeout() time.Duration {
	if hc == nil || hc.Disabled {
		return 0
	}
	interval := hc.Interval
	if interval == 0 {
		interval = 2 * time.Second
	}
	timeout := hc.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	startPeriod := hc.StartPeriod
	retries := hc.Retries
	if retries == 0 {
		retries = 3
	}
	return startPeriod + time.Duration(retries)*(interval+timeout)
}

type PortMapping struct {
	Host      int
	Container int
}

type Volume struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}
