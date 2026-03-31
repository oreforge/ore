package handler

import "context"

type ctxKey int

const (
	ctxKeySpecPath ctxKey = iota
)

func WithSpecPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, ctxKeySpecPath, path)
}
