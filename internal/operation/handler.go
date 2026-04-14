package operation

import (
	"context"
	"encoding/json"
	"log/slog"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

type bufferHandler struct {
	buf   *LogBuffer
	level slog.Level
	attrs []slog.Attr
}

func newBufferHandler(buf *LogBuffer, level slog.Level) *bufferHandler {
	return &bufferHandler{buf: buf, level: level}
}

func (h *bufferHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *bufferHandler) Handle(_ context.Context, r slog.Record) error {
	entry := map[string]any{
		"time":  r.Time.UTC().Format(timeFormat),
		"level": r.Level.String(),
		"msg":   r.Message,
	}

	for _, a := range h.attrs {
		entry[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		entry[a.Key] = a.Value.Any()
		return true
	})

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	h.buf.Append(line)
	return nil
}

func (h *bufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	combined = append(combined, attrs...)
	return &bufferHandler{buf: h.buf, level: h.level, attrs: combined}
}

func (h *bufferHandler) WithGroup(_ string) slog.Handler {
	return h
}

type teeHandler struct {
	handlers []slog.Handler
}

func NewTeeHandler(handlers ...slog.Handler) slog.Handler {
	return &teeHandler{handlers: handlers}
}

func (t *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range t.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (t *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &teeHandler{handlers: handlers}
}

func (t *teeHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &teeHandler{handlers: handlers}
}
