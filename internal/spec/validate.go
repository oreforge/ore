package spec

import (
	"fmt"
	"strconv"
	"strings"
)

func Validate(s *Network) error {
	if s.Network == "" {
		return fmt.Errorf("validation: network name is required")
	}

	if len(s.Servers) == 0 {
		return fmt.Errorf("validation: at least one server is required")
	}

	names := make(map[string]bool, len(s.Servers)+len(s.Services))
	for i, srv := range s.Servers {
		if srv.Name == "" {
			return fmt.Errorf("validation: servers[%d].name is required", i)
		}
		if names[srv.Name] {
			return fmt.Errorf("validation: duplicate name %q", srv.Name)
		}
		names[srv.Name] = true

		if srv.Dir == "" {
			return fmt.Errorf("validation: servers[%d] (%s): dir is required", i, srv.Name)
		}

		if srv.Software == "" {
			return fmt.Errorf("validation: servers[%d] (%s): software is required", i, srv.Name)
		}

		if err := validateSoftwareFormat(srv.Software); err != nil {
			return fmt.Errorf("validation: servers[%d] (%s): %w", i, srv.Name, err)
		}

		for j, p := range srv.Ports {
			if _, err := ParsePort(p); err != nil {
				return fmt.Errorf("validation: servers[%d] (%s): ports[%d]: %w", i, srv.Name, j, err)
			}
		}

		for j, vol := range srv.Volumes {
			if vol.Name == "" {
				return fmt.Errorf("validation: servers[%d] (%s): volumes[%d].name is required", i, srv.Name, j)
			}
			if vol.Target == "" {
				return fmt.Errorf("validation: servers[%d] (%s): volumes[%d].target is required", i, srv.Name, j)
			}
		}

		if err := validateHealthCheck(srv.HealthCheck, fmt.Sprintf("servers[%d] (%s)", i, srv.Name)); err != nil {
			return err
		}
	}

	for i, svc := range s.Services {
		if svc.Name == "" {
			return fmt.Errorf("validation: services[%d].name is required", i)
		}
		if names[svc.Name] {
			return fmt.Errorf("validation: duplicate name %q", svc.Name)
		}
		names[svc.Name] = true

		if svc.Image == "" {
			return fmt.Errorf("validation: services[%d] (%s): image is required", i, svc.Name)
		}

		if err := validateImageFormat(svc.Image); err != nil {
			return fmt.Errorf("validation: services[%d] (%s): %w", i, svc.Name, err)
		}

		for j, p := range svc.Ports {
			if _, err := ParsePort(p); err != nil {
				return fmt.Errorf("validation: services[%d] (%s): ports[%d]: %w", i, svc.Name, j, err)
			}
		}

		for j, vol := range svc.Volumes {
			if vol.Name == "" {
				return fmt.Errorf("validation: services[%d] (%s): volumes[%d].name is required", i, svc.Name, j)
			}
			if vol.Target == "" {
				return fmt.Errorf("validation: services[%d] (%s): volumes[%d].target is required", i, svc.Name, j)
			}
		}

		if err := validateHealthCheck(svc.HealthCheck, fmt.Sprintf("services[%d] (%s)", i, svc.Name)); err != nil {
			return err
		}
	}

	ResolveDependencyConditions(s)

	if err := validateDependencies(s); err != nil {
		return err
	}

	if err := validateNoCycles(s); err != nil {
		return err
	}

	return nil
}

func validateHealthCheck(hc *HealthCheck, context string) error {
	if hc == nil || hc.Disabled {
		return nil
	}
	if hc.Interval < 0 {
		return fmt.Errorf("validation: %s: healthcheck.interval must be positive", context)
	}
	if hc.Timeout < 0 {
		return fmt.Errorf("validation: %s: healthcheck.timeout must be positive", context)
	}
	if hc.StartPeriod < 0 {
		return fmt.Errorf("validation: %s: healthcheck.startPeriod must be positive", context)
	}
	if hc.Retries < 0 {
		return fmt.Errorf("validation: %s: healthcheck.retries must be positive", context)
	}
	return nil
}

func ParsePort(s string) (PortMapping, error) {
	parts := strings.SplitN(s, ":", 2)

	if len(parts) == 1 {
		p, err := parsePortNumber(parts[0])
		if err != nil {
			return PortMapping{}, err
		}
		return PortMapping{Host: p, Container: p}, nil
	}

	host, err := parsePortNumber(parts[0])
	if err != nil {
		return PortMapping{}, fmt.Errorf("host port: %w", err)
	}
	container, err := parsePortNumber(parts[1])
	if err != nil {
		return PortMapping{}, fmt.Errorf("container port: %w", err)
	}

	return PortMapping{Host: host, Container: container}, nil
}

func parsePortNumber(s string) (int, error) {
	p, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port %d out of range (1-65535)", p)
	}
	return p, nil
}

func validateSoftwareFormat(sw string) error {
	parts := strings.SplitN(sw, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("software %q must be in format name:version", sw)
	}
	return nil
}

func validateImageFormat(img string) error {
	parts := strings.SplitN(img, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("image %q must be in format name:tag (e.g. postgres:16)", img)
	}
	return nil
}
