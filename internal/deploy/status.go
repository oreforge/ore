package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/oreforge/ore/internal/spec"
)

type NetworkStatus struct {
	Network  string         `json:"network" doc:"Network name"`
	Servers  []ServerStatus `json:"servers" doc:"Status of each server in the network"`
	Services []ServerStatus `json:"services,omitempty" doc:"Status of each service in the network"`
}

type ServerStatus struct {
	Name      string          `json:"name" doc:"Server name from ore.yaml"`
	Container ContainerStatus `json:"container" doc:"Runtime status"`
}

type ContainerStatus struct {
	Name         string         `json:"name" doc:"Server name"`
	State        ContainerState `json:"state" doc:"Server state (running, exited, etc.)"`
	Health       HealthState    `json:"health" doc:"Health check status"`
	Image        string         `json:"image" doc:"Image tag"`
	Ports        []PortBinding  `json:"ports,omitempty" doc:"Exposed port mappings"`
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

	for _, srv := range s.Servers {
		ss := ServerStatus{
			Name:      srv.Name,
			Container: d.inspectContainer(ctx, ContainerName(&srv)),
		}
		status.Servers = append(status.Servers, ss)
	}

	for _, svc := range s.Services {
		ss := ServerStatus{
			Name:      svc.Name,
			Container: d.inspectContainer(ctx, ServiceContainerName(&svc)),
		}
		status.Services = append(status.Services, ss)
	}

	return status, nil
}

func (d *Deployer) inspectContainer(ctx context.Context, name string) ContainerStatus {
	cs := ContainerStatus{
		Name:  name,
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
