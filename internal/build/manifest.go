package build

import (
	"encoding/json"
	"os"
	"time"
)

type Manifest struct {
	Binaries map[string]BinaryEntry `json:"binaries"`
	Builds   map[string]Entry       `json:"builds"`
}

type BinaryEntry struct {
	SoftwareID string    `json:"software_id"`
	Filename   string    `json:"filename"`
	SHA256     string    `json:"sha256"`
	URL        string    `json:"url"`
	Size       int64     `json:"size_bytes"`
	CachedAt   time.Time `json:"cached_at"`
}

type Entry struct {
	ServerName string    `json:"server_name"`
	ImageTag   string    `json:"image_tag"`
	CacheKey   string    `json:"cache_key"`
	SoftwareID string    `json:"software_id"`
	BuiltAt    time.Time `json:"built_at"`
	DurationMs int64     `json:"duration_ms"`
}

type Metadata struct {
	ServerName   string    `json:"server_name"`
	SoftwareID   string    `json:"software_id"`
	ArtifactURL  string    `json:"artifact_url"`
	ImageTag     string    `json:"image_tag"`
	CacheKey     string    `json:"cache_key"`
	Runtime      string    `json:"runtime"`
	BinaryCached bool      `json:"binary_cached"`
	StartedAt    time.Time `json:"started_at"`
	DurationMs   int64     `json:"duration_ms"`
}

func NewManifest() *Manifest {
	return &Manifest{
		Binaries: make(map[string]BinaryEntry),
		Builds:   make(map[string]Entry),
	}
}

func LoadManifest(path string) *Manifest {
	data, err := os.ReadFile(path)
	if err != nil {
		return NewManifest()
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return NewManifest()
	}

	if m.Binaries == nil {
		m.Binaries = make(map[string]BinaryEntry)
	}
	if m.Builds == nil {
		m.Builds = make(map[string]Entry)
	}

	return &m
}

func (m *Manifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
