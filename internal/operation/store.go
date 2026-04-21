package operation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

var (
	ErrConflict = errors.New("operation conflicts with another in-progress operation for this project")
	ErrNotFound = errors.New("operation not found")
	ErrFinished = errors.New("operation already finished")
)

const completedTTL = 10 * time.Minute

type projectLocks struct {
	exclusive string
	targets   map[string]string
}

type Store struct {
	mu     sync.Mutex
	ops    map[string]*Operation
	locks  map[string]*projectLocks
	logger *slog.Logger
	stopGC context.CancelFunc
}

func NewStore(logger *slog.Logger) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{
		ops:    make(map[string]*Operation),
		locks:  make(map[string]*projectLocks),
		logger: logger,
		stopGC: cancel,
	}
	go s.gc(ctx)
	return s
}

func (s *Store) Stop() {
	s.stopGC()
}

type SubmitRequest struct {
	Project   string
	Action    string
	Target    string
	Targets   []string
	Exclusive bool
	LogLevel  slog.Level
	Logger    *slog.Logger
	Fn        func(ctx context.Context, logger *slog.Logger) error
}

func (s *Store) Submit(req SubmitRequest) (*Operation, error) {
	if !req.Exclusive && len(req.Targets) == 0 {
		return nil, errors.New("operation must be exclusive or specify at least one target")
	}

	s.mu.Lock()

	pl := s.locks[req.Project]
	if pl == nil {
		pl = &projectLocks{targets: make(map[string]string)}
		s.locks[req.Project] = pl
	}

	if req.Exclusive {
		if pl.exclusive != "" || len(pl.targets) > 0 {
			s.mu.Unlock()
			return nil, ErrConflict
		}
	} else {
		if pl.exclusive != "" {
			s.mu.Unlock()
			return nil, ErrConflict
		}
		for _, t := range req.Targets {
			if _, busy := pl.targets[t]; busy {
				s.mu.Unlock()
				return nil, ErrConflict
			}
		}
	}

	now := time.Now()
	buf := newLogBuffer(defaultBufferCap)
	ctx, cancel := context.WithCancel(context.Background())

	op := &Operation{
		ID:        newID(),
		Project:   req.Project,
		Action:    req.Action,
		Target:    req.Target,
		targets:   append([]string(nil), req.Targets...),
		exclusive: req.Exclusive,
		status:    StatusPending,
		createdAt: now,
		cancel:    cancel,
		log:       buf,
	}

	s.ops[op.ID] = op
	if req.Exclusive {
		pl.exclusive = op.ID
	} else {
		for _, t := range req.Targets {
			pl.targets[t] = op.ID
		}
	}
	s.mu.Unlock()

	bufHandler := newBufferHandler(buf, req.LogLevel)
	logger := slog.New(NewTeeHandler(bufHandler, req.Logger.Handler()))

	go s.run(ctx, op, logger, req.Fn)

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
	if pl, ok := s.locks[op.Project]; ok {
		if op.exclusive {
			if pl.exclusive == op.ID {
				pl.exclusive = ""
			}
		} else {
			for _, t := range op.targets {
				if pl.targets[t] == op.ID {
					delete(pl.targets, t)
				}
			}
		}
		if pl.exclusive == "" && len(pl.targets) == 0 {
			delete(s.locks, op.Project)
		}
	}
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
	pl, ok := s.locks[project]
	if !ok {
		return nil, false
	}
	if pl.exclusive != "" {
		return s.ops[pl.exclusive], true
	}
	for _, id := range pl.targets {
		if op, ok := s.ops[id]; ok {
			return op, true
		}
	}
	return nil, false
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
