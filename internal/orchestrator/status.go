package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/oreforge/ore/internal/spec"
)

type NetworkStatus struct {
	Network string         `json:"network"`
	Servers []ServerStatus `json:"servers"`
}

type ServerStatus struct {
	Name     string            `json:"name"`
	Replicas []ContainerStatus `json:"replicas"`
}

type ContainerStatus struct {
	Name         string         `json:"name"`
	State        ContainerState `json:"state"`
	Health       HealthState    `json:"health"`
	Image        string         `json:"image"`
	Ports        []PortBinding  `json:"ports,omitempty"`
	StartedAt    time.Time      `json:"started_at,omitempty"`
	Uptime       time.Duration  `json:"uptime,omitempty"`
	RestartCount int            `json:"restart_count"`
	ExitCode     int            `json:"exit_code"`
	Memory       int64          `json:"memory_bytes"`
	CPUs         float64        `json:"cpus"`
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

func (s ContainerState) String() string {
	switch s {
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

func (s ContainerState) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", s.String())), nil
}

type HealthState int

const (
	HealthNone HealthState = iota
	HealthStarting
	HealthHealthy
	HealthUnhealthy
)

func (h HealthState) String() string {
	switch h {
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

func (h HealthState) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", h.String())), nil
}

type PortBinding struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

func (p PortBinding) String() string {
	return fmt.Sprintf("%d→%d/%s", p.HostPort, p.ContainerPort, p.Protocol)
}

func (o *Orchestrator) Status(ctx context.Context, s *spec.NetworkSpec) (*NetworkStatus, error) {
	status := &NetworkStatus{
		Network: s.Network,
		Servers: make([]ServerStatus, 0, len(s.Servers)),
	}

	for _, srv := range s.Servers {
		ss := ServerStatus{
			Name:     srv.Name,
			Replicas: make([]ContainerStatus, 0),
		}

		for _, name := range ContainerNames(&srv) {
			cs := o.inspectContainer(ctx, name)
			ss.Replicas = append(ss.Replicas, cs)
		}

		status.Servers = append(status.Servers, ss)
	}

	return status, nil
}

func (o *Orchestrator) inspectContainer(ctx context.Context, name string) ContainerStatus {
	cs := ContainerStatus{
		Name:  name,
		State: StateNotFound,
	}

	info, err := o.docker.ContainerInspect(ctx, name)
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
		cs.Memory = info.HostConfig.Memory
		cs.CPUs = float64(info.HostConfig.NanoCPUs) / 1e9

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

	return cs
}
