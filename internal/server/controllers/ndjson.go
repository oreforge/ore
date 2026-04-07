package controllers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
)

const ndjsonDesc = "Streams progress as NDJSON (application/x-ndjson). Final line contains {\"done\":true} with an optional \"error\" field."

func streamOperation(w http.ResponseWriter, logLevel slog.Level, serverLogger *slog.Logger, fn func(*slog.Logger) error) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := w.(http.Flusher)

	ndjson := newNDJSONHandler(w, flusher, logLevel)
	logger := slog.New(newTeeHandler(ndjson, serverLogger.Handler()))

	opErr := fn(logger)

	result := map[string]any{"done": true}
	if opErr != nil {
		result["error"] = opErr.Error()
	}
	line, _ := json.Marshal(result)
	line = append(line, '\n')
	_, _ = w.Write(line)
	if flusher != nil {
		flusher.Flush()
	}
}

type ndjsonHandler struct {
	w       io.Writer
	flusher http.Flusher
	mu      sync.Mutex
	level   slog.Level
	attrs   []slog.Attr
}

func newNDJSONHandler(w io.Writer, flusher http.Flusher, level slog.Level) *ndjsonHandler {
	return &ndjsonHandler{w: w, flusher: flusher, level: level}
}

func (h *ndjsonHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *ndjsonHandler) Handle(_ context.Context, r slog.Record) error {
	entry := map[string]any{
		"time":  r.Time.UTC().Format("2006-01-02T15:04:05.000Z"),
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

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, err := h.w.Write(line); err != nil {
		return err
	}
	if h.flusher != nil {
		h.flusher.Flush()
	}
	return nil
}

func (h *ndjsonHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	combined = append(combined, attrs...)
	return &ndjsonHandler{
		w:       h.w,
		flusher: h.flusher,
		level:   h.level,
		attrs:   combined,
	}
}

func (h *ndjsonHandler) WithGroup(_ string) slog.Handler {
	return h
}

type teeHandler struct {
	handlers []slog.Handler
}

func newTeeHandler(handlers ...slog.Handler) *teeHandler {
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
