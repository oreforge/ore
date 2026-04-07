package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

const (
	labelManaged = "ore.managed"
	labelNetwork = "ore.network"
	labelServer  = "ore.server"
)

func StartContainer(ctx context.Context, client docker.Client, srv *spec.Server, containerName, imageTag, networkName, dataBind string, logger *slog.Logger) error {
	if err := stopAndRemove(ctx, client, containerName, logger); err != nil {
		return err
	}

	logger.Debug("creating container", "name", containerName, "image", imageTag)

	env := buildEnvList(srv)

	containerConfig := &container.Config{
		Image:     imageTag,
		Env:       env,
		Tty:       true,
		OpenStdin: true,
		Labels: map[string]string{
			labelManaged: "true",
			labelNetwork: networkName,
			labelServer:  srv.Name,
		},
	}

	hostConfig := &container.HostConfig{
		Init: new(true),
		RestartPolicy: container.RestartPolicy{
			Name:              container.RestartPolicyOnFailure,
			MaximumRetryCount: 3,
		},
	}

	if err := bindPorts(srv.Ports, containerConfig, hostConfig); err != nil {
		return err
	}

	if srv.Memory != "" {
		mem, err := parseMemory(srv.Memory)
		if err != nil {
			return fmt.Errorf("parsing memory %q: %w", srv.Memory, err)
		}
		hostConfig.Memory = mem
	}

	if srv.CPU != "" {
		cpuNanos, err := parseCPU(srv.CPU)
		if err != nil {
			return fmt.Errorf("parsing cpu %q: %w", srv.CPU, err)
		}
		hostConfig.NanoCPUs = cpuNanos
	}

	if dataBind != "" {
		logger.Debug("bind-mounting data dir", "host", dataBind, "container", "/data")
		hostConfig.Binds = append(hostConfig.Binds, dataBind+":/data")
	}

	for _, vol := range srv.Volumes {
		volName := volumeName(networkName, containerName, vol.Name)
		hostConfig.Binds = append(hostConfig.Binds, volName+":"+vol.Target)
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {
				Aliases: []string{srv.Name},
			},
		},
	}

	resp, err := client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("creating container %s: %w", containerName, err)
	}

	if err := client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container %s: %w", containerName, err)
	}

	logger.Info("container started", "name", containerName)
	return nil
}

func StartServiceContainer(ctx context.Context, client docker.Client, svc *spec.Service, containerName, networkName string, logger *slog.Logger) error {
	if err := stopAndRemove(ctx, client, containerName, logger); err != nil {
		return err
	}

	logger.Debug("creating service container", "name", containerName, "image", svc.Image)

	containerConfig := &container.Config{
		Image:     svc.Image,
		Env:       sortedEnv(svc.Env),
		Tty:       true,
		OpenStdin: true,
		Labels: map[string]string{
			labelManaged: "true",
			labelNetwork: networkName,
			labelServer:  svc.Name,
		},
	}

	hostConfig := &container.HostConfig{
		Init: new(true),
		RestartPolicy: container.RestartPolicy{
			Name:              container.RestartPolicyOnFailure,
			MaximumRetryCount: 3,
		},
	}

	if err := bindPorts(svc.Ports, containerConfig, hostConfig); err != nil {
		return err
	}

	for _, vol := range svc.Volumes {
		volName := volumeName(networkName, containerName, vol.Name)
		hostConfig.Binds = append(hostConfig.Binds, volName+":"+vol.Target)
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {
				Aliases: []string{svc.Name},
			},
		},
	}

	resp, err := client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("creating service container %s: %w", containerName, err)
	}

	if err := client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting service container %s: %w", containerName, err)
	}

	logger.Info("service container started", "name", containerName)
	return nil
}

func bindPorts(ports []string, containerCfg *container.Config, hostCfg *container.HostConfig) error {
	for _, p := range ports {
		pm, err := spec.ParsePort(p)
		if err != nil {
			return fmt.Errorf("parsing port %q: %w", p, err)
		}
		containerPort := nat.Port(fmt.Sprintf("%d/tcp", pm.Container))
		if containerCfg.ExposedPorts == nil {
			containerCfg.ExposedPorts = nat.PortSet{}
		}
		containerCfg.ExposedPorts[containerPort] = struct{}{}
		if hostCfg.PortBindings == nil {
			hostCfg.PortBindings = nat.PortMap{}
		}
		hostCfg.PortBindings[containerPort] = append(
			hostCfg.PortBindings[containerPort],
			nat.PortBinding{HostPort: fmt.Sprintf("%d", pm.Host)},
		)
	}
	return nil
}

func sortedEnv(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+m[k])
	}
	return env
}

func buildEnvList(srv *spec.Server) []string {
	var env []string
	if srv.Memory != "" {
		env = append(env, "ORE_MEMORY="+srv.Memory)
	}
	env = append(env, sortedEnv(srv.Env)...)
	return env
}

func ContainerName(srv *spec.Server) string {
	return srv.Name
}

func ServiceContainerName(svc *spec.Service) string {
	return svc.Name
}

func StopContainer(ctx context.Context, client docker.Client, containerName string, logger *slog.Logger) error {
	logger.Info("stopping container", "name", containerName)
	return stopAndRemove(ctx, client, containerName, logger)
}

func stopAndRemove(ctx context.Context, client docker.Client, name string, logger *slog.Logger) error {
	stopErr := client.ContainerStop(ctx, name, container.StopOptions{Timeout: new(60)})
	if stopErr != nil && !cerrdefs.IsNotFound(stopErr) {
		logger.Debug("graceful stop failed, force removing", "name", name, "error", stopErr)
	}

	err := client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("removing container %s: %w", name, err)
	}
	return nil
}

func listOreContainers(ctx context.Context, client docker.Client, networkName string) ([]container.Summary, error) {
	return client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", labelManaged+"=true"),
			filters.Arg("label", labelNetwork+"="+networkName),
		),
	})
}

func StopAllOreContainers(ctx context.Context, client docker.Client, networkName string, logger *slog.Logger) error {
	containers, err := listOreContainers(ctx, client, networkName)
	if err != nil {
		return err
	}

	for _, c := range containers {
		name := c.ID
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if err := stopAndRemove(ctx, client, name, logger); err != nil {
			logger.Warn("failed to stop orphaned container", "name", name, "error", err)
		}
	}
	return nil
}

func WaitForRunning(ctx context.Context, client docker.Client, containerName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("container %s did not reach running state within %s", containerName, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := client.ContainerInspect(ctx, containerName)
			if err != nil {
				if cerrdefs.IsNotFound(err) {
					return fmt.Errorf("container %s was removed unexpectedly", containerName)
				}
				continue
			}

			if info.State.Running {
				return nil
			}

			if info.State.Status == "exited" {
				return fmt.Errorf("container %s exited with code %d", containerName, info.State.ExitCode)
			}
		}
	}
}
