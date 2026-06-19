package operation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(testLogger())
	t.Cleanup(s.Stop)
	return s
}

func waitForStatus(t *testing.T, op *Operation, want Status) {
	t.Helper()
	require.Eventually(t, func() bool {
		return op.Snapshot().Status == want
	}, 2*time.Second, 5*time.Millisecond)
}

func blockingFn(release <-chan struct{}) func(context.Context, *slog.Logger) error {
	return func(ctx context.Context, _ *slog.Logger) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestSubmitRequiresExclusiveOrTargets(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Action:  ActionUp,
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})

	require.Error(t, err)
	assert.Nil(t, op)
	assert.Contains(t, err.Error(), "must be exclusive or specify at least one target")
}

func TestSubmitConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		first SubmitRequest
		next  SubmitRequest
	}{
		{
			name:  "exclusive_blocks_exclusive",
			first: SubmitRequest{Project: "p", Exclusive: true},
			next:  SubmitRequest{Project: "p", Exclusive: true},
		},
		{
			name:  "exclusive_blocks_target",
			first: SubmitRequest{Project: "p", Exclusive: true},
			next:  SubmitRequest{Project: "p", Targets: []string{"a"}},
		},
		{
			name:  "target_blocks_exclusive",
			first: SubmitRequest{Project: "p", Targets: []string{"a"}},
			next:  SubmitRequest{Project: "p", Exclusive: true},
		},
		{
			name:  "same_target_conflicts",
			first: SubmitRequest{Project: "p", Targets: []string{"a", "b"}},
			next:  SubmitRequest{Project: "p", Targets: []string{"b"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newTestStore(t)
			release := make(chan struct{})
			defer close(release)

			first := tt.first
			first.Logger = testLogger()
			first.Fn = blockingFn(release)

			op1, err := s.Submit(first)
			require.NoError(t, err)
			require.NotNil(t, op1)
			waitForStatus(t, op1, StatusRunning)

			next := tt.next
			next.Logger = testLogger()
			next.Fn = blockingFn(release)

			op2, err := s.Submit(next)
			require.ErrorIs(t, err, ErrConflict)
			assert.Nil(t, op2)
		})
	}
}

func TestSubmitDisjointTargetsSucceed(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	release := make(chan struct{})
	defer close(release)

	op1, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      blockingFn(release),
	})
	require.NoError(t, err)
	require.NotNil(t, op1)
	waitForStatus(t, op1, StatusRunning)

	op2, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"b"},
		Logger:  testLogger(),
		Fn:      blockingFn(release),
	})
	require.NoError(t, err)
	require.NotNil(t, op2)
	waitForStatus(t, op2, StatusRunning)

	assert.NotEqual(t, op1.ID, op2.ID)
}

func TestRunCompleted(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Target:  "a",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)
	waitForStatus(t, op, StatusCompleted)

	assert.Empty(t, op.Snapshot().Error)

	op2, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)
	require.NotNil(t, op2)
}

func TestRunFailed(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return errors.New("boom") },
	})
	require.NoError(t, err)
	waitForStatus(t, op, StatusFailed)

	assert.Equal(t, "boom", op.Snapshot().Error)
}

func TestRunCancelled(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn: func(ctx context.Context, _ *slog.Logger) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	require.NoError(t, err)
	waitForStatus(t, op, StatusRunning)

	require.True(t, op.Cancel())
	waitForStatus(t, op, StatusCancelled)
}

func TestGet(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)

	got, ok := s.Get(op.ID)
	assert.True(t, ok)
	assert.Equal(t, op, got)

	unknown, ok := s.Get("nope")
	assert.False(t, ok)
	assert.Nil(t, unknown)
}

func TestList(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	opA, err := s.Submit(SubmitRequest{
		Project: "a",
		Targets: []string{"t"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)

	opB, err := s.Submit(SubmitRequest{
		Project: "b",
		Targets: []string{"t"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)

	all := s.List("")
	assert.Len(t, all, 2)

	onlyA := s.List("a")
	require.Len(t, onlyA, 1)
	assert.Equal(t, opA.ID, onlyA[0].ID)

	onlyB := s.List("b")
	require.Len(t, onlyB, 1)
	assert.Equal(t, opB.ID, onlyB[0].ID)
}

func TestCancelErrors(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	err := s.Cancel("missing")
	assert.ErrorIs(t, err, ErrNotFound)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)
	waitForStatus(t, op, StatusCompleted)

	err = s.Cancel(op.ID)
	assert.ErrorIs(t, err, ErrFinished)
}

func TestCancelRunning(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn: func(ctx context.Context, _ *slog.Logger) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	require.NoError(t, err)
	waitForStatus(t, op, StatusRunning)

	require.NoError(t, s.Cancel(op.ID))
	waitForStatus(t, op, StatusCancelled)
}

func TestActiveForProject(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	release := make(chan struct{})

	op, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      blockingFn(release),
	})
	require.NoError(t, err)
	waitForStatus(t, op, StatusRunning)

	active, ok := s.ActiveForProject("p")
	require.True(t, ok)
	assert.Equal(t, op.ID, active.ID)

	none, ok := s.ActiveForProject("other")
	assert.False(t, ok)
	assert.Nil(t, none)

	close(release)
	waitForStatus(t, op, StatusCompleted)

	require.Eventually(t, func() bool {
		_, ok := s.ActiveForProject("p")
		return !ok
	}, 2*time.Second, 5*time.Millisecond)
}

func TestEvictKeepsRunningAndRecent(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	release := make(chan struct{})
	defer close(release)

	running, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"a"},
		Logger:  testLogger(),
		Fn:      blockingFn(release),
	})
	require.NoError(t, err)
	waitForStatus(t, running, StatusRunning)

	done, err := s.Submit(SubmitRequest{
		Project: "p",
		Targets: []string{"b"},
		Logger:  testLogger(),
		Fn:      func(context.Context, *slog.Logger) error { return nil },
	})
	require.NoError(t, err)
	waitForStatus(t, done, StatusCompleted)

	s.evict()

	_, ok := s.Get(running.ID)
	assert.True(t, ok)
	_, ok = s.Get(done.ID)
	assert.True(t, ok)
}
