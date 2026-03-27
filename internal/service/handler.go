package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/oreforge/ore/api/orev1"
	"github.com/oreforge/ore/internal/engine"
	"github.com/oreforge/ore/internal/orchestrator"
)

type handler struct {
	orev1.UnimplementedOreServiceServer
	projectsDir string
	listenHost  string
	logLevel    slog.Level
}

func newHandler(projectsDir, listenHost string, logLevel slog.Level) *handler {
	return &handler{
		projectsDir: projectsDir,
		listenHost:  listenHost,
		logLevel:    logLevel,
	}
}

func projectFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-ore-project")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (h *handler) specPath(ctx context.Context) (string, error) {
	name := projectFromContext(ctx)
	if name == "" {
		return "", status.Error(codes.InvalidArgument, "no project specified (use 'ore projects use <name>')")
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) || name == ".." {
		return "", status.Error(codes.InvalidArgument, "invalid project name")
	}
	p := filepath.Join(h.projectsDir, name, "ore.yaml")
	if _, err := os.Stat(p); err != nil {
		return "", status.Errorf(codes.NotFound, "project %q not found", name)
	}
	return p, nil
}

func (h *handler) Up(req *orev1.UpRequest, stream grpc.ServerStreamingServer[orev1.UpResponse]) error {
	sp, err := h.specPath(stream.Context())
	if err != nil {
		return err
	}
	eng := engine.NewLocal(newStreamLogger(wrapStream[orev1.UpResponse](stream), h.logLevel), sp)
	return eng.Up(stream.Context(), req.GetNoCache())
}

func (h *handler) Down(_ *orev1.DownRequest, stream grpc.ServerStreamingServer[orev1.DownResponse]) error {
	sp, err := h.specPath(stream.Context())
	if err != nil {
		return err
	}
	eng := engine.NewLocal(newStreamLogger(wrapStream[orev1.DownResponse](stream), h.logLevel), sp)
	return eng.Down(stream.Context())
}

func (h *handler) Build(req *orev1.BuildRequest, stream grpc.ServerStreamingServer[orev1.BuildResponse]) error {
	sp, err := h.specPath(stream.Context())
	if err != nil {
		return err
	}
	eng := engine.NewLocal(newStreamLogger(wrapStream[orev1.BuildResponse](stream), h.logLevel), sp)
	return eng.Build(stream.Context(), req.GetNoCache())
}

func (h *handler) Status(ctx context.Context, _ *orev1.StatusRequest) (*orev1.StatusResponse, error) {
	sp, err := h.specPath(ctx)
	if err != nil {
		return nil, err
	}
	eng := engine.NewLocal(slog.Default(), sp)
	s, err := eng.Status(ctx)
	if err != nil {
		return nil, err
	}
	return statusToProto(s), nil
}

func (h *handler) Prune(req *orev1.PruneRequest, stream grpc.ServerStreamingServer[orev1.PruneResponse]) error {
	sp, err := h.specPath(stream.Context())
	if err != nil {
		return err
	}
	target, err := pruneTargetFromProto(req.GetTarget())
	if err != nil {
		return err
	}
	eng := engine.NewLocal(newStreamLogger(wrapStream[orev1.PruneResponse](stream), h.logLevel), sp)
	return eng.Prune(stream.Context(), target)
}

func (h *handler) Clean(req *orev1.CleanRequest, stream grpc.ServerStreamingServer[orev1.CleanResponse]) error {
	sp, err := h.specPath(stream.Context())
	if err != nil {
		return err
	}
	target, err := cleanTargetFromProto(req.GetTarget())
	if err != nil {
		return err
	}
	eng := engine.NewLocal(newStreamLogger(wrapStream[orev1.CleanResponse](stream), h.logLevel), sp)
	return eng.Clean(stream.Context(), target)
}

func (h *handler) Console(ctx context.Context, req *orev1.ConsoleRequest) (*orev1.ConsoleResponse, error) {
	sp, err := h.specPath(ctx)
	if err != nil {
		return nil, err
	}
	addr, nonce, err := startConsoleProxy(sp, h.listenHost, req.GetServerName(), int(req.GetReplica()))
	if err != nil {
		return nil, err
	}
	return &orev1.ConsoleResponse{Addr: addr, Nonce: nonce}, nil
}

func (h *handler) ListProjects(_ context.Context, _ *orev1.ListProjectsRequest) (*orev1.ListProjectsResponse, error) {
	entries, err := os.ReadDir(h.projectsDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "reading projects directory: %v", err)
	}

	var projects []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		specFile := filepath.Join(h.projectsDir, e.Name(), "ore.yaml")
		if _, statErr := os.Stat(specFile); statErr == nil {
			projects = append(projects, e.Name())
		}
	}
	sort.Strings(projects)

	return &orev1.ListProjectsResponse{Projects: projects}, nil
}

func statusToProto(s *orchestrator.NetworkStatus) *orev1.StatusResponse {
	resp := &orev1.StatusResponse{
		Network: s.Network,
		Servers: make([]*orev1.ServerStatus, len(s.Servers)),
	}

	for i, srv := range s.Servers {
		ps := &orev1.ServerStatus{
			Name:     srv.Name,
			Replicas: make([]*orev1.ContainerStatus, len(srv.Replicas)),
		}
		for j, c := range srv.Replicas {
			ps.Replicas[j] = &orev1.ContainerStatus{
				Name:          c.Name,
				State:         c.State.String(),
				Health:        c.Health.String(),
				Image:         c.Image,
				Ports:         portsToProto(c.Ports),
				StartedAtUnix: c.StartedAt.Unix(),
				UptimeSeconds: int64(c.Uptime.Seconds()),
				RestartCount:  int32(c.RestartCount),
				ExitCode:      int32(c.ExitCode),
				MemoryBytes:   c.Memory,
				Cpus:          c.CPUs,
			}
		}
		resp.Servers[i] = ps
	}

	return resp
}

func portsToProto(ports []orchestrator.PortBinding) []*orev1.PortBinding {
	if len(ports) == 0 {
		return nil
	}
	result := make([]*orev1.PortBinding, len(ports))
	for i, p := range ports {
		result[i] = &orev1.PortBinding{
			HostPort:      int32(p.HostPort),
			ContainerPort: int32(p.ContainerPort),
			Protocol:      p.Protocol,
		}
	}
	return result
}

func pruneTargetFromProto(t orev1.PruneTarget) (engine.PruneTarget, error) {
	switch t {
	case orev1.PruneTarget_PRUNE_TARGET_ALL:
		return engine.PruneAll, nil
	case orev1.PruneTarget_PRUNE_TARGET_CONTAINERS:
		return engine.PruneContainers, nil
	case orev1.PruneTarget_PRUNE_TARGET_IMAGES:
		return engine.PruneImages, nil
	case orev1.PruneTarget_PRUNE_TARGET_VOLUMES:
		return engine.PruneVolumes, nil
	case orev1.PruneTarget_PRUNE_TARGET_UNSPECIFIED:
		return 0, status.Error(codes.InvalidArgument, "prune target is required")
	default:
		return 0, status.Errorf(codes.InvalidArgument, "unknown prune target: %d", t)
	}
}

func cleanTargetFromProto(t orev1.CleanTarget) (engine.CleanTarget, error) {
	switch t {
	case orev1.CleanTarget_CLEAN_TARGET_ALL:
		return engine.CleanAll, nil
	case orev1.CleanTarget_CLEAN_TARGET_CACHE:
		return engine.CleanCache, nil
	case orev1.CleanTarget_CLEAN_TARGET_BUILDS:
		return engine.CleanBuilds, nil
	case orev1.CleanTarget_CLEAN_TARGET_UNSPECIFIED:
		return 0, status.Error(codes.InvalidArgument, "clean target is required")
	default:
		return 0, status.Errorf(codes.InvalidArgument, "unknown clean target: %d", t)
	}
}
