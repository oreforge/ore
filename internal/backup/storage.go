package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type StorageObject struct {
	Key       string
	SizeBytes int64
}

type Backend interface {
	Name() string
	Put(ctx context.Context, key string) (io.WriteCloser, func() (StorageObject, error), error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (StorageObject, error)
	Delete(ctx context.Context, key string) error
	URI(key string) string
}

type LocalBackend struct {
	root string
}

func NewLocalBackend(root string) (*LocalBackend, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating storage root %s: %w", root, err)
	}
	return &LocalBackend{root: root}, nil
}

func (l *LocalBackend) Name() string { return "local" }

func (l *LocalBackend) URI(key string) string {
	return "file://" + filepath.Join(l.root, key)
}

func (l *LocalBackend) path(key string) string { return filepath.Join(l.root, key) }

func (l *LocalBackend) Put(ctx context.Context, key string) (io.WriteCloser, func() (StorageObject, error), error) {
	final := l.path(key)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return nil, nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(final), filepath.Base(key)+".*.tmp")
	if err != nil {
		return nil, nil, err
	}
	w := &atomicFile{f: tmp, tmpPath: tmp.Name(), finalPath: final}
	commit := func() (StorageObject, error) {
		info, cmErr := w.commit()
		if cmErr != nil {
			return StorageObject{}, cmErr
		}
		return StorageObject{Key: key, SizeBytes: info.Size()}, nil
	}
	return w, commit, nil
}

func (l *LocalBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	f, err := os.Open(l.path(key))
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (l *LocalBackend) Stat(ctx context.Context, key string) (StorageObject, error) {
	info, err := os.Stat(l.path(key))
	if err != nil {
		return StorageObject{}, err
	}
	return StorageObject{Key: key, SizeBytes: info.Size()}, nil
}

func (l *LocalBackend) Delete(ctx context.Context, key string) error {
	err := os.Remove(l.path(key))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

type atomicFile struct {
	f         *os.File
	tmpPath   string
	finalPath string
	closed    bool
}

func (a *atomicFile) Write(p []byte) (int, error) { return a.f.Write(p) }

func (a *atomicFile) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	_ = a.f.Close()
	return nil
}

func (a *atomicFile) commit() (os.FileInfo, error) {
	if a.closed {
		return nil, errors.New("file already closed before commit")
	}
	if err := a.f.Sync(); err != nil {
		_ = a.f.Close()
		_ = os.Remove(a.tmpPath)
		a.closed = true
		return nil, err
	}
	if err := a.f.Close(); err != nil {
		_ = os.Remove(a.tmpPath)
		a.closed = true
		return nil, err
	}
	a.closed = true
	if err := os.Rename(a.tmpPath, a.finalPath); err != nil {
		_ = os.Remove(a.tmpPath)
		return nil, err
	}
	return os.Stat(a.finalPath)
}
