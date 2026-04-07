package spec

import (
	"fmt"
	"sort"
	"strings"
)

type StartGroup struct {
	Servers  []string
	Services []string
}

func ResolveDependencyConditions(s *Network) {
	healthChecks := collectHealthChecks(s)

	resolve := func(deps []Dependency) {
		for i := range deps {
			if deps[i].Condition != "" {
				continue
			}
			hc := healthChecks[deps[i].Name]
			if hc != nil && !hc.Disabled {
				deps[i].Condition = ConditionHealthy
			} else {
				deps[i].Condition = ConditionStarted
			}
		}
	}

	for i := range s.Servers {
		resolve(s.Servers[i].DependsOn)
	}
	for i := range s.Services {
		resolve(s.Services[i].DependsOn)
	}
}

func validateDependencies(s *Network) error {
	names := make(map[string]bool, len(s.Servers)+len(s.Services))
	for _, srv := range s.Servers {
		names[srv.Name] = true
	}
	for _, svc := range s.Services {
		names[svc.Name] = true
	}

	healthChecks := collectHealthChecks(s)

	check := func(owner string, deps []Dependency) error {
		for _, dep := range deps {
			if dep.Name == "" {
				return fmt.Errorf("validation: %s: depends_on entry missing name", owner)
			}
			if dep.Name == owner {
				return fmt.Errorf("validation: %s: depends_on cannot reference itself", owner)
			}
			if !names[dep.Name] {
				return fmt.Errorf("validation: %s: depends_on references unknown name %q", owner, dep.Name)
			}
			if dep.Condition != "" && dep.Condition != ConditionStarted && dep.Condition != ConditionHealthy {
				return fmt.Errorf("validation: %s: depends_on %q has invalid condition %q (must be %q or %q)", owner, dep.Name, dep.Condition, ConditionStarted, ConditionHealthy)
			}
			if dep.Condition == ConditionHealthy {
				hc := healthChecks[dep.Name]
				if hc == nil || hc.Disabled {
					return fmt.Errorf("validation: %s: depends_on %q has condition %q but target has no healthcheck", owner, dep.Name, ConditionHealthy)
				}
			}
		}
		return nil
	}

	for _, srv := range s.Servers {
		if err := check(srv.Name, srv.DependsOn); err != nil {
			return err
		}
	}
	for _, svc := range s.Services {
		if err := check(svc.Name, svc.DependsOn); err != nil {
			return err
		}
	}

	return nil
}

func validateNoCycles(s *Network) error {
	adj, inDegree := buildGraph(s)

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if visited != len(inDegree) {
		var cycleNodes []string
		for name, deg := range inDegree {
			if deg > 0 {
				cycleNodes = append(cycleNodes, name)
			}
		}
		sort.Strings(cycleNodes)
		return fmt.Errorf("validation: circular dependency detected involving: %s", strings.Join(cycleNodes, ", "))
	}

	return nil
}

func TopologicalOrder(s *Network) []StartGroup {
	isService := make(map[string]bool, len(s.Services))
	for _, svc := range s.Services {
		isService[svc.Name] = true
	}

	adj, inDegree := buildGraph(s)

	var groups []StartGroup
	for len(inDegree) > 0 {
		var wave []string
		for name, deg := range inDegree {
			if deg == 0 {
				wave = append(wave, name)
			}
		}
		if len(wave) == 0 {
			break
		}
		sort.Strings(wave)

		group := StartGroup{}
		for _, name := range wave {
			delete(inDegree, name)
			if isService[name] {
				group.Services = append(group.Services, name)
			} else {
				group.Servers = append(group.Servers, name)
			}
			for _, neighbor := range adj[name] {
				inDegree[neighbor]--
			}
		}
		groups = append(groups, group)
	}

	return groups
}

func buildGraph(s *Network) (adj map[string][]string, inDegree map[string]int) {
	adj = make(map[string][]string)
	inDegree = make(map[string]int, len(s.Servers)+len(s.Services))

	for _, srv := range s.Servers {
		if _, ok := inDegree[srv.Name]; !ok {
			inDegree[srv.Name] = 0
		}
		for _, dep := range srv.DependsOn {
			adj[dep.Name] = append(adj[dep.Name], srv.Name)
			inDegree[srv.Name]++
		}
	}
	for _, svc := range s.Services {
		if _, ok := inDegree[svc.Name]; !ok {
			inDegree[svc.Name] = 0
		}
		for _, dep := range svc.DependsOn {
			adj[dep.Name] = append(adj[dep.Name], svc.Name)
			inDegree[svc.Name]++
		}
	}

	return adj, inDegree
}

func collectHealthChecks(s *Network) map[string]*HealthCheck {
	healthChecks := make(map[string]*HealthCheck, len(s.Servers)+len(s.Services))
	for i := range s.Servers {
		healthChecks[s.Servers[i].Name] = s.Servers[i].HealthCheck
	}
	for i := range s.Services {
		healthChecks[s.Services[i].Name] = s.Services[i].HealthCheck
	}
	return healthChecks
}
