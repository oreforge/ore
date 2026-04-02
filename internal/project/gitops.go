package project

import (
	"context"
	"time"

	"github.com/oreforge/ore/internal/spec"
)

func (m *Manager) StartPolling() {
	ctx, cancel := context.WithCancel(context.Background())
	m.pollCancel = cancel

	names, err := m.List()
	if err != nil {
		m.logger.Error("failed to list projects for polling", "error", err)
		return
	}

	for _, name := range names {
		specPath, err := m.Resolve(name)
		if err != nil {
			continue
		}
		s, err := spec.Load(specPath)
		if err != nil {
			m.logger.Warn("failed to load spec for polling", "project", name, "error", err)
			continue
		}
		if s.GitOps == nil || !s.GitOps.Poll.Enabled {
			continue
		}

		interval := s.GitOps.Poll.Interval
		if interval <= 0 {
			interval = defaultPollInterval
		}

		m.pollWg.Add(1)
		go m.poll(ctx, name, interval)
	}
}

func (m *Manager) StopPolling() {
	if m.pollCancel != nil {
		m.pollCancel()
	}
	m.pollWg.Wait()
}

func (m *Manager) poll(ctx context.Context, name string, interval time.Duration) {
	defer m.pollWg.Done()

	logger := m.logger.With("project", name)
	logger.Info("gitops polling started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Deploy(ctx, name); err != nil {
				logger.Error("gitops deploy failed", "error", err)
			}
		}
	}
}

func (m *Manager) Shutdown(ctx context.Context) {
	names, err := m.List()
	if err != nil {
		m.logger.Error("failed to list projects for shutdown", "error", err)
		return
	}
	for _, name := range names {
		m.logger.Info("stopping project", "project", name)
		if err := m.Down(ctx, name, m.logger); err != nil {
			m.logger.Error("failed to stop project", "project", name, "error", err)
		}
	}
}
