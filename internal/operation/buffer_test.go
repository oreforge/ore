package operation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogBufferAppendRead(t *testing.T) {
	t.Parallel()

	b := newLogBuffer(defaultBufferCap)
	const n = 5
	for i := range n {
		b.Append([]byte{byte('a' + i)})
	}

	lines, next, done := b.Read(0)
	assert.Len(t, lines, n)
	assert.Equal(t, n, next)
	assert.False(t, done)

	lines, next, done = b.Read(n)
	assert.Empty(t, lines)
	assert.Equal(t, n, next)
	assert.False(t, done)
}

func TestLogBufferRingEviction(t *testing.T) {
	t.Parallel()

	b := newLogBuffer(2)
	b.Append([]byte("1"))
	b.Append([]byte("2"))
	b.Append([]byte("3"))

	lines, next, _ := b.Read(0)
	require.Len(t, lines, 2)
	assert.Equal(t, "2", string(lines[0]))
	assert.Equal(t, "3", string(lines[1]))
	assert.Equal(t, 3, next)
}

func TestLogBufferWaitUnblocksOnAppend(t *testing.T) {
	t.Parallel()

	b := newLogBuffer(defaultBufferCap)

	go func() {
		time.Sleep(10 * time.Millisecond)
		b.Append([]byte("hi"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	lines, next, done := b.Wait(ctx, 0)
	require.Len(t, lines, 1)
	assert.Equal(t, "hi", string(lines[0]))
	assert.Equal(t, 1, next)
	assert.False(t, done)
}

func TestLogBufferWaitReturnsOnCancel(t *testing.T) {
	t.Parallel()

	b := newLogBuffer(defaultBufferCap)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lines, _, done := b.Wait(ctx, 0)
	assert.Empty(t, lines)
	assert.False(t, done)
	assert.Error(t, ctx.Err())
}

func TestLogBufferCloseReportsDone(t *testing.T) {
	t.Parallel()

	b := newLogBuffer(defaultBufferCap)
	b.Close()

	_, _, done := b.Read(0)
	assert.True(t, done)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, done = b.Wait(ctx, 0)
	assert.True(t, done)
}
