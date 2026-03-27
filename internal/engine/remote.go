package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/oreforge/ore/api/orev1"
	"github.com/oreforge/ore/internal/orchestrator"
)

type Remote struct {
	conn    *grpc.ClientConn
	client  orev1.OreServiceClient
	addr    string
	project string
}

func NewRemote(addr, token, project string) (*Remote, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(tokenCredentials{token: token}))
	}

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to ored at %s: %w", addr, err)
	}

	return &Remote{
		conn:    conn,
		client:  orev1.NewOreServiceClient(conn),
		addr:    addr,
		project: project,
	}, nil
}

func (r *Remote) withProject(ctx context.Context) context.Context {
	if r.project == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "x-ore-project", r.project)
}

func (r *Remote) Up(ctx context.Context, noCache bool) error {
	stream, err := r.client.Up(r.withProject(ctx), &orev1.UpRequest{NoCache: noCache})
	if err != nil {
		return err
	}
	return drainLogs(func() (*orev1.LogEntry, error) {
		m, e := stream.Recv()
		if e != nil {
			return nil, e
		}
		return m.GetLog(), nil
	})
}

func (r *Remote) Down(ctx context.Context) error {
	stream, err := r.client.Down(r.withProject(ctx), &orev1.DownRequest{})
	if err != nil {
		return err
	}
	return drainLogs(func() (*orev1.LogEntry, error) {
		m, e := stream.Recv()
		if e != nil {
			return nil, e
		}
		return m.GetLog(), nil
	})
}

func (r *Remote) Build(ctx context.Context, noCache bool) error {
	stream, err := r.client.Build(r.withProject(ctx), &orev1.BuildRequest{NoCache: noCache})
	if err != nil {
		return err
	}
	return drainLogs(func() (*orev1.LogEntry, error) {
		m, e := stream.Recv()
		if e != nil {
			return nil, e
		}
		return m.GetLog(), nil
	})
}

func (r *Remote) Status(ctx context.Context) (*orchestrator.NetworkStatus, error) {
	resp, err := r.client.Status(r.withProject(ctx), &orev1.StatusRequest{})
	if err != nil {
		return nil, err
	}
	return statusFromProto(resp), nil
}

func (r *Remote) Prune(ctx context.Context, target PruneTarget) error {
	pt, err := pruneTargetToProto(target)
	if err != nil {
		return err
	}
	stream, err := r.client.Prune(r.withProject(ctx), &orev1.PruneRequest{Target: pt})
	if err != nil {
		return err
	}
	return drainLogs(func() (*orev1.LogEntry, error) {
		m, e := stream.Recv()
		if e != nil {
			return nil, e
		}
		return m.GetLog(), nil
	})
}

func (r *Remote) Clean(ctx context.Context, target CleanTarget) error {
	ct, err := cleanTargetToProto(target)
	if err != nil {
		return err
	}
	stream, err := r.client.Clean(r.withProject(ctx), &orev1.CleanRequest{Target: ct})
	if err != nil {
		return err
	}
	return drainLogs(func() (*orev1.LogEntry, error) {
		m, e := stream.Recv()
		if e != nil {
			return nil, e
		}
		return m.GetLog(), nil
	})
}

func (r *Remote) Console(ctx context.Context, serverName string, replica int) error {
	resp, err := r.client.Console(r.withProject(ctx), &orev1.ConsoleRequest{
		ServerName: serverName,
		Replica:    int32(replica),
	})
	if err != nil {
		return err
	}

	consoleAddr := resp.GetAddr()
	_, port, splitErr := net.SplitHostPort(consoleAddr)
	if splitErr == nil {
		remoteHost, _, _ := net.SplitHostPort(r.addr)
		if remoteHost != "" {
			consoleAddr = net.JoinHostPort(remoteHost, port)
		}
	}

	return rawConsole(consoleAddr, resp.GetNonce())
}

func (r *Remote) Close() error {
	return r.conn.Close()
}

func (r *Remote) ListProjects(ctx context.Context) ([]string, error) {
	resp, err := r.client.ListProjects(ctx, &orev1.ListProjectsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetProjects(), nil
}

func drainLogs(recv func() (*orev1.LogEntry, error)) error {
	logger := slog.Default()
	for {
		entry, err := recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if entry == nil {
			continue
		}

		level := parseLogLevel(entry.GetLevel())
		attrs := make([]any, 0, len(entry.GetAttrs())*2)
		for _, a := range entry.GetAttrs() {
			attrs = append(attrs, a.GetKey(), a.GetValue())
		}
		logger.Log(context.Background(), level, entry.GetMessage(), attrs...)
	}
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func statusFromProto(resp *orev1.StatusResponse) *orchestrator.NetworkStatus {
	status := &orchestrator.NetworkStatus{
		Network: resp.GetNetwork(),
		Servers: make([]orchestrator.ServerStatus, len(resp.GetServers())),
	}

	for i, srv := range resp.GetServers() {
		ss := orchestrator.ServerStatus{
			Name:     srv.GetName(),
			Replicas: make([]orchestrator.ContainerStatus, len(srv.GetReplicas())),
		}
		for j, c := range srv.GetReplicas() {
			ss.Replicas[j] = orchestrator.ContainerStatus{
				Name:         c.GetName(),
				State:        parseContainerState(c.GetState()),
				Health:       parseHealthState(c.GetHealth()),
				Image:        c.GetImage(),
				Ports:        portsFromProto(c.GetPorts()),
				StartedAt:    time.Unix(c.GetStartedAtUnix(), 0),
				Uptime:       time.Duration(c.GetUptimeSeconds()) * time.Second,
				RestartCount: int(c.GetRestartCount()),
				ExitCode:     int(c.GetExitCode()),
				Memory:       c.GetMemoryBytes(),
				CPUs:         c.GetCpus(),
			}
		}
		status.Servers[i] = ss
	}

	return status
}

func portsFromProto(ports []*orev1.PortBinding) []orchestrator.PortBinding {
	if len(ports) == 0 {
		return nil
	}
	result := make([]orchestrator.PortBinding, len(ports))
	for i, p := range ports {
		result[i] = orchestrator.PortBinding{
			HostPort:      int(p.GetHostPort()),
			ContainerPort: int(p.GetContainerPort()),
			Protocol:      p.GetProtocol(),
		}
	}
	return result
}

func parseContainerState(s string) orchestrator.ContainerState {
	switch s {
	case "created":
		return orchestrator.StateCreated
	case "running":
		return orchestrator.StateRunning
	case "exited":
		return orchestrator.StateExited
	case "paused":
		return orchestrator.StatePaused
	case "dead":
		return orchestrator.StateDead
	default:
		return orchestrator.StateNotFound
	}
}

func parseHealthState(s string) orchestrator.HealthState {
	switch s {
	case "starting":
		return orchestrator.HealthStarting
	case "healthy":
		return orchestrator.HealthHealthy
	case "unhealthy":
		return orchestrator.HealthUnhealthy
	default:
		return orchestrator.HealthNone
	}
}

func pruneTargetToProto(t PruneTarget) (orev1.PruneTarget, error) {
	switch t {
	case PruneAll:
		return orev1.PruneTarget_PRUNE_TARGET_ALL, nil
	case PruneContainers:
		return orev1.PruneTarget_PRUNE_TARGET_CONTAINERS, nil
	case PruneImages:
		return orev1.PruneTarget_PRUNE_TARGET_IMAGES, nil
	case PruneVolumes:
		return orev1.PruneTarget_PRUNE_TARGET_VOLUMES, nil
	default:
		return 0, fmt.Errorf("unknown prune target: %d", t)
	}
}

func cleanTargetToProto(t CleanTarget) (orev1.CleanTarget, error) {
	switch t {
	case CleanAll:
		return orev1.CleanTarget_CLEAN_TARGET_ALL, nil
	case CleanCache:
		return orev1.CleanTarget_CLEAN_TARGET_CACHE, nil
	case CleanBuilds:
		return orev1.CleanTarget_CLEAN_TARGET_BUILDS, nil
	default:
		return 0, fmt.Errorf("unknown clean target: %d", t)
	}
}
