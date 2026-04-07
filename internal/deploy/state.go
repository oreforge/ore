package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type State struct {
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

func NewDeployState() *State {
	return &State{
		Servers:  make(map[string]ServerState),
		Services: make(map[string]ServiceState),
	}
}

func LoadState(dir string) *State {
	path := filepath.Join(dir, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return NewDeployState()
	}

	var s State
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

func SaveState(dir string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "state.json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
