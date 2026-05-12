package bead

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Identity is the typed context payload for caller identity metadata.
type Identity struct {
	Subject string
}

type identityContextKey struct{}
type traceContextKey struct{}

// WithIdentity stores the caller identity on ctx.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, identityContextKey{}, id)
}

// IdentityFromContext extracts the caller identity from ctx.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	id, ok := ctx.Value(identityContextKey{}).(Identity)
	return id, ok
}

// WithTrace stores a trace span on ctx.
func WithTrace(ctx context.Context, span trace.Span) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceContextKey{}, span)
}

// TraceFromContext extracts a trace span from ctx.
func TraceFromContext(ctx context.Context) (trace.Span, bool) {
	if ctx == nil {
		return nil, false
	}
	span, ok := ctx.Value(traceContextKey{}).(trace.Span)
	return span, ok
}
