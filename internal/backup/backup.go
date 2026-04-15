package backup

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/oklog/ulid"

	"github.com/oreforge/ore/internal/volumes"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Kind string

const (
	KindManual     Kind = "manual"
	KindScheduled  Kind = "scheduled"
	KindPreRestore Kind = "pre-restore"
)

type Backup struct {
	ID          string       `json:"id"`
	Project     string       `json:"project"`
	Volume      string       `json:"volume"`
	LogicalName string       `json:"logicalName,omitempty"`
	Kind        Kind         `json:"kind"`
	Status      Status       `json:"status"`
	CreatedAt   time.Time    `json:"createdAt"`
	FinishedAt  *time.Time   `json:"finishedAt,omitempty"`
	Error       string       `json:"error,omitempty"`
	SizeBytes   int64        `json:"sizeBytes"`
	Compressed  int64        `json:"compressed"`
	Algorithm   string       `json:"algorithm"`
	Checksum    string       `json:"checksum,omitempty"`
	Storage     []StorageRef `json:"storage"`
	Verified    *time.Time   `json:"verified,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
}

type StorageRef struct {
	Backend string `json:"backend"`
	URI     string `json:"uri"`
}

var (
	ErrNotFound = errors.New("backup not found")
	ErrConflict = errors.New("backup already exists")
)

type Filter struct {
	Project string
	Volume  string
	Status  Status
}

type Service struct {
	volumes     *volumes.Service
	index       Index
	backend     Backend
	snapshotter Snapshotter
	log         *slog.Logger

	entropy io.Reader
	entMu   sync.Mutex
}

type Options struct {
	Volumes     *volumes.Service
	Index       Index
	Backend     Backend
	Snapshotter Snapshotter
	Logger      *slog.Logger
}

func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		volumes:     opts.Volumes,
		index:       opts.Index,
		backend:     opts.Backend,
		snapshotter: opts.Snapshotter,
		log:         logger,
		entropy:     ulid.Monotonic(cryptorand.Reader, 0),
	}
}

func NewLocalService(root string, vs *volumes.Service, snap Snapshotter, logger *slog.Logger) (*Service, error) {
	idx, err := NewFSIndex(root)
	if err != nil {
		return nil, err
	}
	local, err := NewLocalBackend(root)
	if err != nil {
		return nil, err
	}
	return NewService(Options{
		Volumes:     vs,
		Index:       idx,
		Backend:     local,
		Snapshotter: snap,
		Logger:      logger,
	}), nil
}

func (s *Service) newID(now time.Time) string {
	s.entMu.Lock()
	defer s.entMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(now), s.entropy).String()
}

type CreateOptions struct {
	Project string
	Volume  string
	Kind    Kind
	Tags    []string
}

func (s *Service) Create(ctx context.Context, logger *slog.Logger, opts CreateOptions) (*Backup, error) {
	vol, err := s.volumes.Inspect(ctx, opts.Volume)
	if err != nil {
		return nil, fmt.Errorf("inspecting volume: %w", err)
	}
	if vol.Project != opts.Project {
		return nil, fmt.Errorf("volume %s does not belong to project %s", opts.Volume, opts.Project)
	}

	now := time.Now().UTC()
	kind := opts.Kind
	if kind == "" {
		kind = KindManual
	}

	b := &Backup{
		ID:          s.newID(now),
		Project:     opts.Project,
		Volume:      vol.Name,
		LogicalName: vol.Logical,
		Kind:        kind,
		Status:      StatusRunning,
		CreatedAt:   now,
		Algorithm:   "tar+zstd",
		Tags:        opts.Tags,
	}
	if err := s.index.Insert(b); err != nil {
		return nil, err
	}

	logger.Info("starting backup", "id", b.ID, "volume", b.Volume)

	if err := s.runSnapshot(ctx, b, logger); err != nil {
		b.Status = StatusFailed
		b.Error = err.Error()
		finished := time.Now().UTC()
		b.FinishedAt = &finished
		_ = s.index.Update(b)
		_ = s.backend.Delete(context.Background(), archiveKey(b))
		return b, err
	}

	b.Status = StatusCompleted
	finished := time.Now().UTC()
	b.FinishedAt = &finished
	b.Storage = []StorageRef{{Backend: s.backend.Name(), URI: s.backend.URI(archiveKey(b))}}
	if err := s.index.Update(b); err != nil {
		return b, err
	}

	logger.Info("backup complete",
		"id", b.ID,
		"raw_bytes", b.SizeBytes,
		"compressed_bytes", b.Compressed,
		"sha256", b.Checksum,
	)
	return b, nil
}

func (s *Service) runSnapshot(ctx context.Context, b *Backup, logger *slog.Logger) error {
	stream, err := s.snapshotter.Snapshot(ctx, b.Volume, logger)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Reader.Close() }()

	key := archiveKey(b)
	wc, commit, err := s.backend.Put(ctx, key)
	if err != nil {
		return fmt.Errorf("opening backend: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = wc.Close()
		}
	}()

	aw, err := NewArchiveWriter(wc)
	if err != nil {
		return err
	}

	if _, err := io.Copy(aw, stream.Reader); err != nil {
		_, _ = aw.Close()
		return fmt.Errorf("streaming archive: %w", err)
	}

	res, err := aw.Close()
	if err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("closing backend writer: %w", err)
	}
	if err := stream.Wait(); err != nil {
		return err
	}
	if _, err := commit(); err != nil {
		return fmt.Errorf("committing archive: %w", err)
	}
	committed = true

	b.SizeBytes = res.RawBytes
	b.Compressed = res.CompressedBytes
	b.Checksum = res.Checksum
	return nil
}

func (s *Service) List(f Filter) []*Backup {
	return s.index.List(f)
}

func (s *Service) Get(id string) (*Backup, error) {
	return s.index.Get(id)
}

func (s *Service) Remove(ctx context.Context, id string) error {
	b, err := s.index.Get(id)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(ctx, archiveKey(b)); err != nil {
		return err
	}
	return s.index.Delete(id)
}

type VerifyResult struct {
	SizeBytes  int64
	Checksum   string
	MatchesRef bool
}

func (s *Service) Verify(ctx context.Context, id string) (*VerifyResult, error) {
	b, err := s.index.Get(id)
	if err != nil {
		return nil, err
	}
	rc, err := s.backend.Get(ctx, archiveKey(b))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	sum, size, err := hashAndCount(rc)
	if err != nil {
		return nil, err
	}

	matches := sum == b.Checksum
	if matches {
		now := time.Now().UTC()
		b.Verified = &now
		_ = s.index.Update(b)
	}
	return &VerifyResult{SizeBytes: size, Checksum: sum, MatchesRef: matches}, nil
}

type RestoreOptions struct {
	KeepPreRestore bool
}

func (s *Service) Restore(ctx context.Context, logger *slog.Logger, id string, opts RestoreOptions) error {
	b, err := s.index.Get(id)
	if err != nil {
		return err
	}
	if b.Status != StatusCompleted {
		return fmt.Errorf("cannot restore backup %s in status %s", id, b.Status)
	}

	vol, err := s.volumes.Inspect(ctx, b.Volume)
	if err != nil {
		return fmt.Errorf("target volume: %w", err)
	}
	if vol.Project != b.Project {
		return fmt.Errorf("target volume %s is in project %s, expected %s", vol.Name, vol.Project, b.Project)
	}

	if opts.KeepPreRestore {
		logger.Info("taking pre-restore safety snapshot", "volume", vol.Name)
		if _, preErr := s.Create(ctx, logger, CreateOptions{
			Project: b.Project,
			Volume:  b.Volume,
			Kind:    KindPreRestore,
			Tags:    []string{"pre-restore", id},
		}); preErr != nil {
			return fmt.Errorf("pre-restore snapshot failed: %w", preErr)
		}
	}

	rc, err := s.backend.Get(ctx, archiveKey(b))
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer func() { _ = rc.Close() }()

	decompressed, err := NewArchiveReader(rc)
	if err != nil {
		return fmt.Errorf("opening archive reader: %w", err)
	}
	defer func() { _ = decompressed.Close() }()

	logger.Info("extracting backup into volume", "id", id, "volume", b.Volume)
	if err := s.snapshotter.Extract(ctx, b.Volume, decompressed, logger); err != nil {
		return fmt.Errorf("restoring volume: %w", err)
	}

	logger.Info("restore complete", "id", id, "volume", b.Volume)
	return nil
}

func archiveKey(b *Backup) string {
	return b.Project + "/" + b.Volume + "/" + b.ID + ".tar.zst"
}

func hashAndCount(r io.Reader) (string, int64, error) {
	hasher := sha256.New()
	var total int64
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = hasher.Write(buf[:n])
			total += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", total, err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), total, nil
}
