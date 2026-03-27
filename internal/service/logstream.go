package service

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc"

	"github.com/oreforge/ore/api/orev1"
)

type logSender interface {
	Send(*orev1.LogEntry) error
	Context() context.Context
}

type wrappedStream[T any] struct {
	stream  grpc.ServerStreamingServer[T]
	wrapper func(*orev1.LogEntry) *T
}

func (w *wrappedStream[T]) Send(entry *orev1.LogEntry) error {
	return w.stream.Send(w.wrapper(entry))
}

func (w *wrappedStream[T]) Context() context.Context {
	return w.stream.Context()
}

func wrapStream[T any](stream grpc.ServerStreamingServer[T]) logSender {
	return &wrappedStream[T]{
		stream: stream,
		wrapper: func(entry *orev1.LogEntry) *T {
			var t T
			switch v := any(&t).(type) {
			case *orev1.UpResponse:
				v.Log = entry
			case *orev1.DownResponse:
				v.Log = entry
			case *orev1.BuildResponse:
				v.Log = entry
			case *orev1.PruneResponse:
				v.Log = entry
			case *orev1.CleanResponse:
				v.Log = entry
			}
			return &t
		},
	}
}

type streamLogHandler struct {
	sender logSender
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func newStreamLogger(sender logSender, level slog.Level) *slog.Logger {
	return slog.New(&streamLogHandler{sender: sender, level: level})
}

func (h *streamLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *streamLogHandler) Handle(_ context.Context, r slog.Record) error {
	entry := &orev1.LogEntry{
		TimeUnixMilli: r.Time.UnixMilli(),
		Level:         r.Level.String(),
		Message:       r.Message,
	}

	prefix := ""
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}

	for _, a := range h.attrs {
		entry.Attrs = append(entry.Attrs, &orev1.LogAttr{
			Key:   prefix + a.Key,
			Value: a.Value.String(),
		})
	}

	r.Attrs(func(a slog.Attr) bool {
		entry.Attrs = append(entry.Attrs, &orev1.LogAttr{
			Key:   prefix + a.Key,
			Value: a.Value.String(),
		})
		return true
	})

	return h.sender.Send(entry)
}

func (h *streamLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &streamLogHandler{sender: h.sender, level: h.level, attrs: newAttrs, groups: h.groups}
}

func (h *streamLogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := make([]string, len(h.groups)+1)
	copy(groups, h.groups)
	groups[len(h.groups)] = name
	return &streamLogHandler{sender: h.sender, level: h.level, attrs: h.attrs, groups: groups}
}
