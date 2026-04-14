package operation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

var (
	ErrConflict = errors.New("operation already in progress for this project")
	ErrNotFound = errors.New("operation not found")
	ErrFinished = errors.New("operation already finished")
)

const completedTTL = 10 * time.Minute

type Store struct {
	mu     sync.Mutex
	ops    map[string]*Operation
	active map[string]string
	logger *slog.Logger
	stopGC context.CancelFunc
}

func NewStore(logger *slog.Logger) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{
		ops:    make(map[string]*Operation),
		active: make(map[string]string),
		logger: logger,
		stopGC: cancel,
	}
	go s.gc(ctx)
	return s
}

func (s *Store) Stop() {
	s.stopGC()
}

func (s *Store) Submit(
	project, action, target string,
	logLevel slog.Level,
	serverLogger *slog.Logger,
	fn func(ctx context.Context, logger *slog.Logger) error,
) (*Operation, error) {
	s.mu.Lock()
	if _, exists := s.active[project]; exists {
		s.mu.Unlock()
		return nil, ErrConflict
	}

	now := time.Now()
	buf := newLogBuffer(defaultBufferCap)
	ctx, cancel := context.WithCancel(context.Background())

	op := &Operation{
		ID:        newID(),
		Project:   project,
		Action:    action,
		Target:    target,
		status:    StatusPending,
		createdAt: now,
		cancel:    cancel,
		log:       buf,
	}

	s.ops[op.ID] = op
	s.active[project] = op.ID
	s.mu.Unlock()

	bufHandler := newBufferHandler(buf, logLevel)
	logger := slog.New(NewTeeHandler(bufHandler, serverLogger.Handler()))

	go s.run(ctx, op, logger, fn)

	return op, nil
}

func (s *Store) run(ctx context.Context, op *Operation, logger *slog.Logger, fn func(context.Context, *slog.Logger) error) {
	op.mu.Lock()
	op.status = StatusRunning
	op.startedAt = time.Now()
	op.mu.Unlock()

	err := fn(ctx, logger)

	op.mu.Lock()
	op.endedAt = time.Now()
	switch {
	case ctx.Err() != nil:
		op.status = StatusCancelled
	case err != nil:
		op.status = StatusFailed
		op.errMsg = err.Error()
	default:
		op.status = StatusCompleted
	}
	op.mu.Unlock()

	s.mu.Lock()
	delete(s.active, op.Project)
	s.mu.Unlock()

	op.log.Close()
}

func (s *Store) Get(id string) (*Operation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.ops[id]
	return op, ok
}

func (s *Store) List(project string) []*Operation {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*Operation
	for _, op := range s.ops {
		if project == "" || op.Project == project {
			result = append(result, op)
		}
	}
	return result
}

func (s *Store) Cancel(id string) error {
	op, ok := s.Get(id)
	if !ok {
		return ErrNotFound
	}
	if !op.Cancel() {
		return ErrFinished
	}
	return nil
}

func (s *Store) ActiveForProject(project string) (*Operation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.active[project]
	if !ok {
		return nil, false
	}
	return s.ops[id], true
}

func (s *Store) gc(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evict()
		}
	}
}

func (s *Store) evict() {
	cutoff := time.Now().Add(-completedTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, op := range s.ops {
		snap := op.Snapshot()
		if op.Done() && snap.EndedAt.Before(cutoff) {
			delete(s.ops, id)
		}
	}
}
