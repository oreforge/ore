package operation

import (
	"context"
	"sync"
)

const defaultBufferCap = 1000

type LogBuffer struct {
	mu       sync.Mutex
	cond     *sync.Cond
	lines    [][]byte
	offset   int
	capacity int
	closed   bool
}

func newLogBuffer(capacity int) *LogBuffer {
	b := &LogBuffer{
		lines:    make([][]byte, 0, capacity),
		capacity: capacity,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *LogBuffer) Append(line []byte) {
	cp := make([]byte, len(line))
	copy(cp, line)

	b.mu.Lock()
	if len(b.lines) >= b.capacity {
		b.lines = b.lines[1:]
	}
	b.lines = append(b.lines, cp)
	b.offset++
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (b *LogBuffer) Read(cursor int) (lines [][]byte, nextCursor int, done bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.readLocked(cursor)
}

func (b *LogBuffer) Wait(ctx context.Context, cursor int) (lines [][]byte, nextCursor int, done bool) {
	stop := context.AfterFunc(ctx, func() { b.cond.Broadcast() })
	defer stop()

	b.mu.Lock()
	defer b.mu.Unlock()

	for {
		lines, nextCursor, done = b.readLocked(cursor)
		if len(lines) > 0 || done || ctx.Err() != nil {
			return lines, nextCursor, done
		}
		b.cond.Wait()
	}
}

func (b *LogBuffer) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (b *LogBuffer) readLocked(cursor int) ([][]byte, int, bool) {
	start := b.offset - len(b.lines)
	if cursor < start {
		cursor = start
	}
	if cursor >= b.offset {
		return nil, b.offset, b.closed
	}
	idx := cursor - start
	out := make([][]byte, len(b.lines)-idx)
	copy(out, b.lines[idx:])
	return out, b.offset, b.closed
}
