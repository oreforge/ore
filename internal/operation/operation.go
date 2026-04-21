package operation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

const (
	ActionUp           = "up"
	ActionDown         = "down"
	ActionBuild        = "build"
	ActionClean        = "clean"
	ActionUpdate       = "update"
	ActionStart        = "start"
	ActionStop         = "stop"
	ActionRestart      = "restart"
	ActionDeploy       = "deploy"
	ActionVolumeRemove = "volume.remove"
)

type Operation struct {
	ID      string
	Project string
	Action  string
	Target  string

	mu        sync.RWMutex
	status    Status
	errMsg    string
	createdAt time.Time
	startedAt time.Time
	endedAt   time.Time

	cancel context.CancelFunc
	log    *LogBuffer
}

type Snapshot struct {
	ID        string
	Project   string
	Action    string
	Target    string
	Status    Status
	Error     string
	CreatedAt time.Time
	StartedAt time.Time
	EndedAt   time.Time
}

func (op *Operation) Snapshot() Snapshot {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return Snapshot{
		ID:        op.ID,
		Project:   op.Project,
		Action:    op.Action,
		Target:    op.Target,
		Status:    op.status,
		Error:     op.errMsg,
		CreatedAt: op.createdAt,
		StartedAt: op.startedAt,
		EndedAt:   op.endedAt,
	}
}

func (op *Operation) Cancel() bool {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.isDoneLocked() || op.cancel == nil {
		return false
	}
	op.cancel()
	return true
}

func (op *Operation) LogBuffer() *LogBuffer {
	return op.log
}

func (op *Operation) Done() bool {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.isDoneLocked()
}

func (op *Operation) ErrorMsg() string {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.errMsg
}

func (op *Operation) isDoneLocked() bool {
	return op.status == StatusCompleted || op.status == StatusFailed || op.status == StatusCancelled
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
