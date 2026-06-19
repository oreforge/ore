package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/software"
	"github.com/oreforge/ore/internal/spec"
)

type NetworkStatus struct {
	Network  string         `json:"network" doc:"Network name"`
	Servers  []ServerStatus `json:"servers" doc:"Status of each server in the network"`
	Services []ServerStatus `json:"services,omitempty" doc:"Status of each service in the network"`
}

type ServerStatus struct {
	Name      string          `json:"name" doc:"Workload name from ore.yaml"`
	Kind      WorkloadKind    `json:"kind" doc:"Workload kind: server (built from software) or service (pre-built image)"`
	Spec      WorkloadSpec    `json:"spec" doc:"Declarative configuration from ore.yaml"`
	Build     *BuildInfo      `json:"build,omitempty" doc:"Resolved build artifact metadata; null for services and for servers with no build on disk"`
	Container ContainerStatus `json:"container" doc:"Live runtime state"`
}

type WorkloadKind string

const (
	KindServer  WorkloadKind = "server"
	KindService WorkloadKind = "service"
)

type WorkloadSpec struct {
	Source      SpecSource         `json:"source" doc:"Where the workload's image comes from"`
	Ports       []spec.PortMapping `json:"ports,omitempty" doc:"Declared port mappings; may differ from container.ports"`
	Memory      string             `json:"memory,omitempty" doc:"Declared memory limit (e.g. 2Gi)"`
	CPU         string             `json:"cpu,omitempty" doc:"Declared CPU limit (e.g. 1)"`
	Env         map[string]string  `json:"env,omitempty" doc:"Declared environment variables"`
	Volumes     []spec.Volume      `json:"volumes,omitempty" doc:"Declared volume mounts"`
	DependsOn   []spec.Dependency  `json:"depends_on,omitempty" doc:"Declared startup dependencies"`
	HealthCheck *spec.HealthCheck  `json:"healthcheck,omitempty" doc:"Declared healthcheck override"`
}

type SpecSource struct {
	Type     string    `json:"type" doc:"Source kind: software (built by ore) or image (pulled from a registry)"`
	Software *Software `json:"software,omitempty" doc:"Populated when type == software"`
	Image    string    `json:"image,omitempty" doc:"Populated when type == image"`
}

type Software struct {
	Raw     string `json:"raw" doc:"Verbatim software field from ore.yaml (e.g. paper:1.20.6)"`
	Name    string `json:"name,omitempty" doc:"Parsed software name (e.g. paper)"`
	Version string `json:"version,omitempty" doc:"Parsed software version (e.g. 1.20.6)"`
}

type BuildInfo struct {
	ImageTag  string    `json:"image_tag" doc:"Image tag the build pipeline produced; may differ from container.image after rollback"`
	CacheKey  string    `json:"cache_key" doc:"Hash of software, version, and server dir contents"`
	BaseImage string    `json:"base_image,omitempty" doc:"Base image the build was layered on (e.g. alpine:3.19)"`
	BuiltAt   time.Time `json:"built_at,omitempty" doc:"When the build completed"`
}

type ContainerStatus struct {
	State        ContainerState `json:"state" doc:"Server state (running, exited, etc.)"`
	Health       HealthState    `json:"health" doc:"Health check status"`
	Image        string         `json:"image" doc:"Image currently running; may differ from build.image_tag"`
	Ports        []PortBinding  `json:"ports,omitempty" doc:"Live host port bindings"`
	StartedAt    time.Time      `json:"started_at,omitempty" doc:"Start time"`
	Uptime       time.Duration  `json:"uptime,omitempty" doc:"Time since started"`
	RestartCount int            `json:"restart_count" doc:"Number of restarts"`
	ExitCode     int            `json:"exit_code" doc:"Last exit code"`
	Resources    ResourceStatus `json:"resources" doc:"Resource limits and usage"`
}

type ResourceStatus struct {
	Memory MemoryStatus `json:"memory" doc:"Memory limits and usage"`
	CPU    CPUStatus    `json:"cpu" doc:"CPU limits and usage"`
}

type MemoryStatus struct {
	UsedBytes  uint64  `json:"used_bytes" doc:"Current memory usage in bytes"`
	LimitBytes int64   `json:"limit_bytes" doc:"Memory limit in bytes (0 = unlimited)"`
	Percent    float64 `json:"percent" doc:"Usage as percentage of limit (0 if unlimited)"`
}

type CPUStatus struct {
	Limit   float64 `json:"limit" doc:"CPU core limit (0 = unlimited)"`
	Percent float64 `json:"percent" doc:"Current CPU usage percentage"`
}

type ContainerState int

const (
	StateNotFound ContainerState = iota
	StateCreated
	StateRunning
	StateExited
	StatePaused
	StateDead
)

func (s *ContainerState) String() string {
	switch *s {
	case StateNotFound:
		return "not found"
	case StateCreated:
		return "created"
	case StateRunning:
		return "running"
	case StateExited:
		return "exited"
	case StatePaused:
		return "paused"
	case StateDead:
		return "dead"
	default:
		return "unknown"
	}
}

func (s *ContainerState) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", s.String())), nil
}

func (s *ContainerState) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "not found":
		*s = StateNotFound
	case "created":
		*s = StateCreated
	case "running":
		*s = StateRunning
	case "exited":
		*s = StateExited
	case "paused":
		*s = StatePaused
	case "dead":
		*s = StateDead
	default:
		*s = StateNotFound
	}
	return nil
}

type HealthState int

const (
	HealthNone HealthState = iota
	HealthStarting
	HealthHealthy
	HealthUnhealthy
)

func (h *HealthState) String() string {
	switch *h {
	case HealthNone:
		return "—"
	case HealthStarting:
		return "starting"
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

func (h *HealthState) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", h.String())), nil
}

func (h *HealthState) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "starting":
		*h = HealthStarting
	case "healthy":
		*h = HealthHealthy
	case "unhealthy":
		*h = HealthUnhealthy
	default:
		*h = HealthNone
	}
	return nil
}

type PortBinding struct {
	HostPort      int    `json:"host_port" doc:"Port on the host"`
	ContainerPort int    `json:"container_port" doc:"Server port"`
	Protocol      string `json:"protocol" doc:"Protocol (tcp/udp)"`
}

func (p PortBinding) String() string {
	return fmt.Sprintf("%d→%d/%s", p.HostPort, p.ContainerPort, p.Protocol)
}

func (d *Deployer) Status(ctx context.Context, s *spec.Network) (*NetworkStatus, error) {
	status := &NetworkStatus{
		Network: s.Network,
		Servers: make([]ServerStatus, 0, len(s.Servers)),
	}

	manifest := d.manifest()

	for i := range s.Servers {
		srv := &s.Servers[i]
		status.Servers = append(status.Servers, ServerStatus{
			Name:      srv.Name,
			Kind:      KindServer,
			Spec:      workloadSpecForServer(srv, d.logger),
			Build:     buildInfoFor(manifest, srv.Name),
			Container: d.inspectContainer(ctx, ContainerName(srv)),
		})
	}

	for i := range s.Services {
		svc := &s.Services[i]
		status.Services = append(status.Services, ServerStatus{
			Name:      svc.Name,
			Kind:      KindService,
			Spec:      workloadSpecForService(svc, d.logger),
			Container: d.inspectContainer(ctx, ServiceContainerName(svc)),
		})
	}

	return status, nil
}

func (d *Deployer) manifest() *build.Manifest {
	if d == nil || d.workDir == nil {
		return nil
	}
	return d.workDir.Manifest()
}

func workloadSpecForServer(srv *spec.Server, logger *slog.Logger) WorkloadSpec {
	return WorkloadSpec{
		Source:      softwareSource(srv.Software),
		Ports:       parsePorts(srv.Ports, srv.Name, logger),
		Memory:      srv.Memory,
		CPU:         srv.CPU,
		Env:         srv.Env,
		Volumes:     srv.Volumes,
		DependsOn:   srv.DependsOn,
		HealthCheck: srv.HealthCheck,
	}
}

func workloadSpecForService(svc *spec.Service, logger *slog.Logger) WorkloadSpec {
	return WorkloadSpec{
		Source:      imageSource(svc.Image),
		Ports:       parsePorts(svc.Ports, svc.Name, logger),
		Env:         svc.Env,
		Volumes:     svc.Volumes,
		DependsOn:   svc.DependsOn,
		HealthCheck: svc.HealthCheck,
	}
}

func softwareSource(raw string) SpecSource {
	src := SpecSource{Type: "software", Software: &Software{Raw: raw}}
	if name, version, err := software.ParseSpec(raw); err == nil {
		src.Software.Name = name
		src.Software.Version = version
	}
	return src
}

func imageSource(image string) SpecSource {
	return SpecSource{Type: "image", Image: image}
}

func parsePorts(ports []string, owner string, logger *slog.Logger) []spec.PortMapping {
	if len(ports) == 0 {
		return nil
	}
	out := make([]spec.PortMapping, 0, len(ports))
	for _, p := range ports {
		pm, err := spec.ParsePort(p)
		if err != nil {
			if logger != nil {
				logger.Debug("skipping unparseable port in status response", "owner", owner, "port", p, "error", err)
			}
			continue
		}
		out = append(out, pm)
	}
	return out
}

func buildInfoFor(manifest *build.Manifest, serverName string) *BuildInfo {
	entry, ok := manifest.LatestBuild(serverName)
	if !ok {
		return nil
	}
	return &BuildInfo{
		ImageTag:  entry.ImageTag,
		CacheKey:  entry.CacheKey,
		BaseImage: entry.BaseImage,
		BuiltAt:   entry.BuiltAt,
	}
}

func (d *Deployer) inspectContainer(ctx context.Context, name string) ContainerStatus {
	cs := ContainerStatus{
		State: StateNotFound,
	}

	info, err := d.docker.ContainerInspect(ctx, name)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return cs
		}
		return cs
	}

	cs.Image = info.Config.Image
	cs.RestartCount = info.RestartCount

	switch info.State.Status {
	case "created":
		cs.State = StateCreated
	case "running":
		cs.State = StateRunning
	case "exited":
		cs.State = StateExited
		cs.ExitCode = info.State.ExitCode
	case "paused":
		cs.State = StatePaused
	case "dead":
		cs.State = StateDead
	}

	if info.State.Health != nil {
		switch info.State.Health.Status {
		case "starting":
			cs.Health = HealthStarting
		case "healthy":
			cs.Health = HealthHealthy
		case "unhealthy":
			cs.Health = HealthUnhealthy
		}
	}

	if info.State.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
			cs.StartedAt = t
			if cs.State == StateRunning {
				cs.Uptime = time.Since(t)
			}
		}
	}

	if info.HostConfig != nil {
		cs.Resources.Memory.LimitBytes = info.HostConfig.Memory
		cs.Resources.CPU.Limit = float64(info.HostConfig.NanoCPUs) / 1e9

		for port, bindings := range info.HostConfig.PortBindings {
			containerPort, _ := strconv.Atoi(strings.Split(string(port), "/")[0])
			proto := "tcp"
			if parts := strings.Split(string(port), "/"); len(parts) == 2 {
				proto = parts[1]
			}
			for _, b := range bindings {
				hostPort, _ := strconv.Atoi(b.HostPort)
				cs.Ports = append(cs.Ports, PortBinding{
					HostPort:      hostPort,
					ContainerPort: containerPort,
					Protocol:      proto,
				})
			}
		}
	}

	if cs.State == StateRunning {
		d.fillRuntimeStats(ctx, name, &cs)
	}

	return cs
}

func (d *Deployer) fillRuntimeStats(ctx context.Context, name string, cs *ContainerStatus) {
	statsReader, err := d.docker.ContainerStatsOneShot(ctx, name)
	if err != nil {
		return
	}
	defer func() { _ = statsReader.Body.Close() }()

	var stats container.StatsResponse
	if err := json.NewDecoder(statsReader.Body).Decode(&stats); err != nil {
		return
	}

	cs.Resources.Memory.UsedBytes = stats.MemoryStats.Usage
	if cs.Resources.Memory.LimitBytes > 0 {
		cs.Resources.Memory.Percent = math.Round(float64(stats.MemoryStats.Usage)/float64(cs.Resources.Memory.LimitBytes)*1000) / 10
	}

	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if systemDelta > 0 && stats.CPUStats.OnlineCPUs > 0 {
		cs.Resources.CPU.Percent = math.Round(cpuDelta/systemDelta*float64(stats.CPUStats.OnlineCPUs)*1000) / 10
	}
}
