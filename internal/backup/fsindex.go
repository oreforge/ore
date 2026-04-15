package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Index interface {
	Insert(b *Backup) error
	Update(b *Backup) error
	Get(id string) (*Backup, error)
	List(f Filter) []*Backup
	Delete(id string) error
}

type FSIndex struct {
	root string

	mu   sync.RWMutex
	data map[string]*Backup
}

func NewFSIndex(root string) (*FSIndex, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating backup root %s: %w", root, err)
	}
	idx := &FSIndex{root: root, data: make(map[string]*Backup)}
	if err := idx.loadAll(); err != nil {
		return nil, err
	}
	return idx, nil
}

func (i *FSIndex) Root() string { return i.root }

func (i *FSIndex) loadAll() error {
	return filepath.WalkDir(i.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		b, readErr := loadSidecar(path)
		if readErr != nil {
			return fmt.Errorf("loading sidecar %s: %w", path, readErr)
		}
		i.mu.Lock()
		i.data[b.ID] = b
		i.mu.Unlock()
		return nil
	})
}

func (i *FSIndex) projectDir(project, volume string) string {
	return filepath.Join(i.root, project, volume)
}

func (i *FSIndex) SidecarPath(b *Backup) string {
	return filepath.Join(i.projectDir(b.Project, b.Volume), b.ID+".json")
}

func (i *FSIndex) ArchivePath(b *Backup) string {
	return filepath.Join(i.projectDir(b.Project, b.Volume), b.ID+".tar.zst")
}

func (i *FSIndex) Insert(b *Backup) error {
	i.mu.Lock()
	if _, exists := i.data[b.ID]; exists {
		i.mu.Unlock()
		return ErrConflict
	}
	i.data[b.ID] = b
	i.mu.Unlock()
	return i.writeSidecar(b)
}

func (i *FSIndex) Update(b *Backup) error {
	i.mu.Lock()
	if _, exists := i.data[b.ID]; !exists {
		i.mu.Unlock()
		return ErrNotFound
	}
	i.data[b.ID] = b
	i.mu.Unlock()
	return i.writeSidecar(b)
}

func (i *FSIndex) Get(id string) (*Backup, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	b, ok := i.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *b
	return &clone, nil
}

func (i *FSIndex) List(f Filter) []*Backup {
	i.mu.RLock()
	defer i.mu.RUnlock()

	out := make([]*Backup, 0, len(i.data))
	for _, b := range i.data {
		if f.Project != "" && b.Project != f.Project {
			continue
		}
		if f.Volume != "" && b.Volume != f.Volume {
			continue
		}
		if f.Status != "" && b.Status != f.Status {
			continue
		}
		clone := *b
		out = append(out, &clone)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.After(out[b].CreatedAt) })
	return out
}

func (i *FSIndex) Delete(id string) error {
	i.mu.Lock()
	b, ok := i.data[id]
	if !ok {
		i.mu.Unlock()
		return ErrNotFound
	}
	delete(i.data, id)
	i.mu.Unlock()

	archivePath := i.ArchivePath(b)
	sidecarPath := i.SidecarPath(b)

	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing archive %s: %w", archivePath, err)
	}
	if err := os.Remove(sidecarPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing sidecar %s: %w", sidecarPath, err)
	}
	return nil
}

func (i *FSIndex) writeSidecar(b *Backup) error {
	dir := i.projectDir(b.Project, b.Volume)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	final := i.SidecarPath(b)
	tmp, err := os.CreateTemp(dir, ".sidecar-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp sidecar: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encoding sidecar: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing sidecar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing sidecar: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("renaming sidecar: %w", err)
	}
	return nil
}

func loadSidecar(path string) (*Backup, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var b Backup
	if err := json.NewDecoder(f).Decode(&b); err != nil {
		return nil, err
	}
	if b.ID == "" {
		return nil, fmt.Errorf("sidecar %s has no ID", path)
	}
	if b.Project == "" || b.Volume == "" {
		return nil, fmt.Errorf("sidecar %s missing project or volume", path)
	}
	return &b, nil
}
