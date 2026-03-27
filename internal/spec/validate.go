package spec

import (
	"fmt"
	"strings"
)

func Validate(s *NetworkSpec) error {
	if s.Network == "" {
		return fmt.Errorf("validation: network name is required")
	}

	if len(s.Servers) == 0 {
		return fmt.Errorf("validation: at least one server is required")
	}

	names := make(map[string]bool, len(s.Servers))
	for i, srv := range s.Servers {
		if srv.Name == "" {
			return fmt.Errorf("validation: servers[%d].name is required", i)
		}
		if names[srv.Name] {
			return fmt.Errorf("validation: duplicate server name %q", srv.Name)
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

		if srv.Port < 0 || srv.Port > 65535 {
			return fmt.Errorf("validation: servers[%d] (%s): port must be 0-65535", i, srv.Name)
		}

		if srv.Replicas < 0 {
			return fmt.Errorf("validation: servers[%d] (%s): replicas must be non-negative", i, srv.Name)
		}

		for j, vol := range srv.Volumes {
			if vol.Name == "" {
				return fmt.Errorf("validation: servers[%d] (%s): volumes[%d].name is required", i, srv.Name, j)
			}
			if vol.Target == "" {
				return fmt.Errorf("validation: servers[%d] (%s): volumes[%d].target is required", i, srv.Name, j)
			}
		}
	}

	return nil
}

func validateSoftwareFormat(sw string) error {
	parts := strings.SplitN(sw, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("software %q must be in format name:version", sw)
	}
	return nil
}
