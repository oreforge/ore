package software

import "time"

type Artifact struct {
	Version string
	URL     string
	SHA256  string
	Runtime Runtime
	Health  HealthCheck
}

type Runtime struct {
	BaseImage  string
	BinaryName string
	BinaryMode int64
	Entrypoint string
	ExtraArgs  string
}

type HealthCheck struct {
	Cmd         string
	Interval    time.Duration
	Timeout     time.Duration
	StartPeriod time.Duration
	Retries     int
}
