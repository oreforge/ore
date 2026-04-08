package dto

import (
	"github.com/oreforge/ore/internal/build"
	"github.com/oreforge/ore/internal/deploy"
	"github.com/oreforge/ore/internal/spec"
)

type AddProjectRequest struct {
	URL  string `json:"url" validate:"required" example:"https://github.com/org/network.git"`
	Name string `json:"name,omitempty" example:"mynetwork"`
}

type ProjectResponse struct {
	Name string `json:"name" example:"mynetwork"`
}

type ProjectListResponse struct {
	Projects []string `json:"projects"`
}

type WebhookInfoResponse struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty" example:"/webhook/mynetwork?secret=abc123"`
	Secret  string `json:"secret,omitempty"`
	Force   bool   `json:"force,omitempty"`
	NoCache bool   `json:"no_cache,omitempty"`
}

type WebhookResponse struct {
	Status  string `json:"status" example:"accepted"`
	Project string `json:"project" example:"mynetwork"`
}

type ProjectDetailResponse struct {
	Name  string        `json:"name" doc:"Project name"`
	Spec  SpecResponse  `json:"spec" doc:"Parsed ore.yaml specification"`
	State StateResponse `json:"state" doc:"Current deploy state"`
}

type StateResponse struct {
	Servers  map[string]StateServer  `json:"servers"`
	Services map[string]StateService `json:"services"`
}

type StateServer struct {
	ImageTag   string `json:"image_tag"`
	ConfigHash string `json:"config_hash"`
}

type StateService struct {
	Image      string `json:"image"`
	ConfigHash string `json:"config_hash"`
}

type SpecResponse struct {
	Network  string        `json:"network"`
	Servers  []SpecServer  `json:"servers"`
	Services []SpecService `json:"services,omitempty"`
	GitOps   *SpecGitOps   `json:"gitops,omitempty"`
}

type SpecServer struct {
	Name        string            `json:"name"`
	Dir         string            `json:"dir"`
	Software    string            `json:"software"`
	Ports       []string          `json:"ports,omitempty"`
	Memory      string            `json:"memory,omitempty"`
	CPU         string            `json:"cpu,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Volumes     []SpecVolume      `json:"volumes,omitempty"`
	HealthCheck *SpecHealthCheck  `json:"healthcheck,omitempty"`
	DependsOn   []SpecDependency  `json:"depends_on,omitempty"`
}

type SpecService struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Ports       []string          `json:"ports,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Volumes     []SpecVolume      `json:"volumes,omitempty"`
	HealthCheck *SpecHealthCheck  `json:"healthcheck,omitempty"`
	DependsOn   []SpecDependency  `json:"depends_on,omitempty"`
}

type SpecVolume struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

type SpecHealthCheck struct {
	Disabled    bool   `json:"disabled,omitempty"`
	Cmd         string `json:"cmd,omitempty"`
	Interval    string `json:"interval,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	StartPeriod string `json:"start_period,omitempty"`
	Retries     int    `json:"retries,omitempty"`
}

type SpecDependency struct {
	Name      string `json:"name"`
	Condition string `json:"condition,omitempty"`
}

type SpecGitOps struct {
	Poll    *SpecGitOpsPoll    `json:"poll,omitempty"`
	Webhook *SpecGitOpsWebhook `json:"webhook,omitempty"`
}

type SpecGitOpsPoll struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval,omitempty"`
}

type SpecGitOpsWebhook struct {
	Enabled bool `json:"enabled"`
	Force   bool `json:"force,omitempty"`
	NoCache bool `json:"no_cache,omitempty"`
}

func NewSpecResponse(s *spec.Network) SpecResponse {
	resp := SpecResponse{
		Network:  s.Network,
		Servers:  make([]SpecServer, len(s.Servers)),
		Services: make([]SpecService, len(s.Services)),
	}

	for i, srv := range s.Servers {
		resp.Servers[i] = SpecServer{
			Name:     srv.Name,
			Dir:      srv.Dir,
			Software: srv.Software,
			Ports:    srv.Ports,
			Memory:   srv.Memory,
			CPU:      srv.CPU,
			Env:      srv.Env,
		}
		for _, v := range srv.Volumes {
			resp.Servers[i].Volumes = append(resp.Servers[i].Volumes, SpecVolume{Name: v.Name, Target: v.Target})
		}
		if srv.HealthCheck != nil {
			resp.Servers[i].HealthCheck = convertHealthCheck(srv.HealthCheck)
		}
		for _, d := range srv.DependsOn {
			resp.Servers[i].DependsOn = append(resp.Servers[i].DependsOn, SpecDependency{Name: d.Name, Condition: string(d.Condition)})
		}
	}

	for i, svc := range s.Services {
		resp.Services[i] = SpecService{
			Name:  svc.Name,
			Image: svc.Image,
			Ports: svc.Ports,
			Env:   svc.Env,
		}
		for _, v := range svc.Volumes {
			resp.Services[i].Volumes = append(resp.Services[i].Volumes, SpecVolume{Name: v.Name, Target: v.Target})
		}
		if svc.HealthCheck != nil {
			resp.Services[i].HealthCheck = convertHealthCheck(svc.HealthCheck)
		}
		for _, d := range svc.DependsOn {
			resp.Services[i].DependsOn = append(resp.Services[i].DependsOn, SpecDependency{Name: d.Name, Condition: string(d.Condition)})
		}
	}

	if s.GitOps != nil {
		resp.GitOps = &SpecGitOps{}
		if s.GitOps.Poll.Enabled || s.GitOps.Poll.Interval > 0 {
			resp.GitOps.Poll = &SpecGitOpsPoll{
				Enabled:  s.GitOps.Poll.Enabled,
				Interval: s.GitOps.Poll.Interval.String(),
			}
		}
		if s.GitOps.Webhook.Enabled {
			resp.GitOps.Webhook = &SpecGitOpsWebhook{
				Enabled: s.GitOps.Webhook.Enabled,
				Force:   s.GitOps.Webhook.Force,
				NoCache: s.GitOps.Webhook.NoCache,
			}
		}
	}

	return resp
}

func convertHealthCheck(hc *spec.HealthCheck) *SpecHealthCheck {
	if hc == nil {
		return nil
	}
	r := &SpecHealthCheck{
		Disabled: hc.Disabled,
		Cmd:      hc.Cmd,
		Retries:  hc.Retries,
	}
	if hc.Interval > 0 {
		r.Interval = hc.Interval.String()
	}
	if hc.Timeout > 0 {
		r.Timeout = hc.Timeout.String()
	}
	if hc.StartPeriod > 0 {
		r.StartPeriod = hc.StartPeriod.String()
	}
	return r
}

func NewStateResponse(s *deploy.State) StateResponse {
	resp := StateResponse{
		Servers:  make(map[string]StateServer, len(s.Servers)),
		Services: make(map[string]StateService, len(s.Services)),
	}
	for k, v := range s.Servers {
		resp.Servers[k] = StateServer{ImageTag: v.ImageTag, ConfigHash: v.ConfigHash}
	}
	for k, v := range s.Services {
		resp.Services[k] = StateService{Image: v.Image, ConfigHash: v.ConfigHash}
	}
	return resp
}

type BuildsResponse = build.Manifest
