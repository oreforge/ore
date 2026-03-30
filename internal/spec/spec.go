package spec

type NetworkSpec struct {
	Network string       `yaml:"network"`
	Servers []ServerSpec `yaml:"servers"`
}

type ServerSpec struct {
	Name     string            `yaml:"name"`
	Dir      string            `yaml:"dir"`
	Software string            `yaml:"software"`
	Port     int               `yaml:"port,omitempty"`
	Memory   string            `yaml:"memory,omitempty"`
	CPU      string            `yaml:"cpu,omitempty"`
	JVMFlags []string          `yaml:"jvmFlags,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Volumes  []VolumeSpec      `yaml:"volumes,omitempty"`
}

type VolumeSpec struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}
