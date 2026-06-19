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
	Name        string            `yaml:"name" json:"name"`
	Dir         string            `yaml:"dir" json:"dir,omitempty"`
	Software    string            `yaml:"software" json:"software,omitempty"`
	Ports       []string          `yaml:"ports,omitempty" json:"ports,omitempty"`
	Memory      string            `yaml:"memory,omitempty" json:"memory,omitempty"`
	CPU         string            `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Volumes     []Volume          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	HealthCheck *HealthCheck      `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`
	DependsOn   []Dependency      `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
}

type Service struct {
	Name        string            `yaml:"name" json:"name"`
	Image       string            `yaml:"image" json:"image,omitempty"`
	Ports       []string          `yaml:"ports,omitempty" json:"ports,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Volumes     []Volume          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	HealthCheck *HealthCheck      `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`
	DependsOn   []Dependency      `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
}

type DependencyCondition string

const (
	ConditionStarted DependencyCondition = "started"
	ConditionHealthy DependencyCondition = "healthy"
)

type Dependency struct {
	Name      string              `yaml:"name" json:"name"`
	Condition DependencyCondition `yaml:"condition,omitempty" json:"condition,omitempty"`
}

type HealthCheck struct {
	Disabled    bool          `yaml:"-" json:"disabled,omitempty"`
	Cmd         string        `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	Interval    time.Duration `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout     time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	StartPeriod time.Duration `yaml:"startPeriod,omitempty" json:"start_period,omitempty"`
	Retries     int           `yaml:"retries,omitempty" json:"retries,omitempty"`
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
	Host      int `json:"host"`
	Container int `json:"container"`
}

type Volume struct {
	Name   string `yaml:"name" json:"name"`
	Target string `yaml:"target" json:"target"`
}
