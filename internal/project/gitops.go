package project

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/oreforge/ore/internal/operation"
	"github.com/oreforge/ore/internal/spec"
)

func (m *Manager) StartPolling() {
	ctx, cancel := context.WithCancel(context.Background())
	m.pollCtx = ctx
	m.pollCancel = cancel

	names, err := m.List()
	if err != nil {
		m.logger.Error("failed to list projects for polling", "error", err)
		return
	}

	for _, name := range names {
		m.startProjectPoll(name)
	}
}

func (m *Manager) StopPolling() {
	if m.pollCancel != nil {
		m.pollCancel()
	}

	m.pollMu.Lock()
	entries := make(map[string]pollEntry, len(m.polls))
	for k, v := range m.polls {
		entries[k] = v
	}
	m.polls = make(map[string]pollEntry)
	m.pollMu.Unlock()

	for _, e := range entries {
		<-e.done
	}
}

func (m *Manager) RestartProjectPoll(name string) {
	m.StopProjectPoll(name)
	m.startProjectPoll(name)
}

func (m *Manager) startProjectPoll(name string) {
	if m.pollCtx == nil {
		return
	}

	specPath, err := m.Resolve(name)
	if err != nil {
		return
	}
	s, err := spec.Load(specPath)
	if err != nil {
		m.logger.Warn("failed to load spec for polling", "project", name, "error", err)
		return
	}
	if s.GitOps == nil || !s.GitOps.Poll.Enabled {
		return
	}

	interval := s.GitOps.Poll.Interval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	ctx, cancel := context.WithCancel(m.pollCtx)
	done := make(chan struct{})

	m.pollMu.Lock()
	m.polls[name] = pollEntry{cancel: cancel, done: done}
	m.pollMu.Unlock()

	go m.poll(ctx, name, interval, done)
}

func (m *Manager) StopProjectPoll(name string) {
	m.pollMu.Lock()
	entry, ok := m.polls[name]
	if ok {
		delete(m.polls, name)
	}
	m.pollMu.Unlock()

	if ok {
		entry.cancel()
		select {
		case <-entry.done:
		case <-time.After(5 * time.Second):
			m.logger.Warn("poll goroutine did not exit in time", "project", name)
		}
	}
}

func (m *Manager) poll(ctx context.Context, name string, interval time.Duration, done chan struct{}) {
	defer close(done)

	logger := m.logger.With("project", name)
	logger.Info("gitops polling started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("gitops polling stopped")
			return
		case <-ticker.C:
			m.syncDeploy(ctx, name)
		}
	}
}

func (m *Manager) TriggerDeploy(name string, opts UpOptions) (string, error) {
	op, err := m.opStore.Submit(name, operation.ActionDeploy, "", slog.LevelInfo, m.logger,
		func(ctx context.Context, logger *slog.Logger) error {
			return m.Deploy(ctx, name, opts)
		})
	if err != nil {
		return "", err
	}
	return op.ID, nil
}

func (m *Manager) syncDeploy(ctx context.Context, name string) {
	logger := m.logger.With("project", name)

	changed, err := m.hasRemoteChanges(ctx, name)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Warn("failed to check for remote changes, skipping deploy", "error", err)
		return
	}
	if !changed {
		logger.Debug("no remote changes detected, skipping deploy")
		return
	}

	if _, triggerErr := m.TriggerDeploy(name, UpOptions{}); triggerErr != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Warn("gitops deploy skipped", "error", triggerErr)
	}
}

func (m *Manager) Shutdown(ctx context.Context) {
	names, err := m.List()
	if err != nil {
		m.logger.Error("failed to list projects for shutdown", "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.logger.Info("stopping project", "project", name)
			if err := m.Down(ctx, name, m.logger); err != nil {
				m.logger.Error("failed to stop project", "project", name, "error", err)
			}
		}()
	}
	wg.Wait()
}
