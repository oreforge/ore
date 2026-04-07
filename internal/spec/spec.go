package spec

import "time"

type Network struct {
	Network  string    `yaml:"network"`
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
	Name     string            `yaml:"name"`
	Dir      string            `yaml:"dir"`
	Software string            `yaml:"software"`
	Ports    []string          `yaml:"ports,omitempty"`
	Memory   string            `yaml:"memory,omitempty"`
	CPU      string            `yaml:"cpu,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Volumes  []Volume          `yaml:"volumes,omitempty"`
}

type Service struct {
	Name    string            `yaml:"name"`
	Image   string            `yaml:"image"`
	Ports   []string          `yaml:"ports,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Volumes []Volume          `yaml:"volumes,omitempty"`
}

type PortMapping struct {
	Host      int
	Container int
}

type Volume struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}
