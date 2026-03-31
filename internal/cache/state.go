package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type DeployState struct {
	Servers  map[string]ServerState  `json:"servers"`
	Services map[string]ServiceState `json:"services"`
}

type ServerState struct {
	ImageTag   string `json:"image_tag"`
	ConfigHash string `json:"config_hash"`
}

type ServiceState struct {
	Image      string `json:"image"`
	ConfigHash string `json:"config_hash"`
}

func NewDeployState() *DeployState {
	return &DeployState{
		Servers:  make(map[string]ServerState),
		Services: make(map[string]ServiceState),
	}
}

func (w *Manager) LoadState() *DeployState {
	path := filepath.Join(w.root, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return NewDeployState()
	}

	var s DeployState
	if err := json.Unmarshal(data, &s); err != nil {
		return NewDeployState()
	}

	if s.Servers == nil {
		s.Servers = make(map[string]ServerState)
	}
	if s.Services == nil {
		s.Services = make(map[string]ServiceState)
	}

	return &s
}

func (w *Manager) SaveState(state *DeployState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.root, "state.json"), data, 0o644)
}
