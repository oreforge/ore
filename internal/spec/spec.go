package spec

import "time"

type NetworkSpec struct {
	Network  string        `yaml:"network"`
	GitOps   *GitOpsSpec   `yaml:"gitops,omitempty"`
	Servers  []ServerSpec  `yaml:"servers"`
	Services []ServiceSpec `yaml:"services,omitempty"`
}

type GitOpsSpec struct {
	Poll GitOpsPoll `yaml:"poll,omitempty"`
}

type GitOpsPoll struct {
	Enabled  bool          `yaml:"enabled,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty"`
}

type ServerSpec struct {
	Name     string            `yaml:"name"`
	Dir      string            `yaml:"dir"`
	Software string            `yaml:"software"`
	Ports    []string          `yaml:"ports,omitempty"`
	Memory   string            `yaml:"memory,omitempty"`
	CPU      string            `yaml:"cpu,omitempty"`
	JVMFlags []string          `yaml:"jvmFlags,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Volumes  []VolumeSpec      `yaml:"volumes,omitempty"`
}

type ServiceSpec struct {
	Name    string            `yaml:"name"`
	Image   string            `yaml:"image"`
	Ports   []string          `yaml:"ports,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Volumes []VolumeSpec      `yaml:"volumes,omitempty"`
}

type PortMapping struct {
	Host      int
	Container int
}

type VolumeSpec struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}
