package orchestrator

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

func StartContainer(ctx context.Context, client docker.Client, srv *spec.ServerSpec, containerName, imageTag, networkName, dataBind string, logger *slog.Logger) error {
	if err := stopAndRemove(ctx, client, containerName, logger); err != nil {
		return err
	}

	logger.Info("creating container", "name", containerName, "image", imageTag)

	containerConfig := &container.Config{
		Image:     imageTag,
		Env:       buildEnvList(srv),
		Tty:       true,
		OpenStdin: true,
		Labels: map[string]string{
			labelManaged: "true",
			labelNetwork: networkName,
			labelServer:  srv.Name,
		},
	}

	init := true
	hostConfig := &container.HostConfig{
		Init: &init,
	}

	if srv.Port > 0 {
		portStr := fmt.Sprintf("%d/tcp", srv.Port)
		port := nat.Port(portStr)
		containerConfig.ExposedPorts = nat.PortSet{port: {}}
		hostConfig.PortBindings = nat.PortMap{
			port: {{HostPort: fmt.Sprintf("%d", srv.Port)}},
		}
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

	hostConfig.RestartPolicy = container.RestartPolicy{
		Name:              container.RestartPolicyOnFailure,
		MaximumRetryCount: 3,
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {},
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

func StopContainer(ctx context.Context, client docker.Client, containerName string, logger *slog.Logger) error {
	logger.Info("stopping container", "name", containerName)
	return stopAndRemove(ctx, client, containerName, logger)
}

func stopAndRemove(ctx context.Context, client docker.Client, name string, logger *slog.Logger) error {
	timeout := 60
	stopErr := client.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
	if stopErr != nil && !cerrdefs.IsNotFound(stopErr) {
		logger.Debug("graceful stop failed, force removing", "name", name, "error", stopErr)
	}

	err := client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("removing container %s: %w", name, err)
	}
	return nil
}

func buildEnvList(srv *spec.ServerSpec) []string {
	var env []string

	if len(srv.JVMFlags) > 0 {
		env = append(env, "ORE_JVM_FLAGS="+strings.Join(srv.JVMFlags, " "))
	}

	keys := make([]string, 0, len(srv.Env))
	for k := range srv.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+srv.Env[k])
	}

	return env
}

func ContainerName(srv *spec.ServerSpec) string {
	return srv.Name
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
		if len(c.Names) == 0 {
			logger.Warn("container has no names, skipping", "id", c.ID)
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
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
