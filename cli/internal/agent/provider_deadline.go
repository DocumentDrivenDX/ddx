package agent

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
)

// DefaultProviderRequestTimeout bounds a single Chat / ChatStream call.
// Defeats RC4 of ddx-0a651925: a stalled TCP socket that has delivered
// headers but stopped emitting body bytes would otherwise pin a goroutine
// until the outer wall-clock (3h) frees it.
const DefaultProviderRequestTimeout = 15 * time.Minute

// DefaultProviderIdleReadTimeout bounds the maximum idle gap between stream
// deltas. When streaming, a socket mid-stream-stall delivers no StreamDelta
// events and so cannot be caught by the event-level idle timer in
// agent_runner.go. This per-stream cap fires regardless.
const DefaultProviderIdleReadTimeout = 5 * time.Minute

// ErrProviderRequestTimeout is returned by the deadline wrapper when a
// per-request bound (wall-clock or idle-read) fires. It is distinct from
// context.DeadlineExceeded so callers can tell a wrapper-triggered timeout
// from a caller-ctx cancel.
var ErrProviderRequestTimeout = errors.New("provider request timeout")

// wrapProviderWithDeadlines returns p decorated with per-request deadlines.
// When p also implements agentlib.StreamingProvider, the returned wrapper
// implements StreamingProvider too (so agentlib.Run still picks the
// streaming code path).
func wrapProviderWithDeadlines(p agentlib.Provider) agentlib.Provider {
	return wrapProviderWithDeadlinesTimeouts(p, DefaultProviderRequestTimeout, DefaultProviderIdleReadTimeout)
}

// wrapProviderWithDeadlinesTimeouts is the explicit-timeout variant used by
// tests so they can exercise the bounds without waiting 15m.
func wrapProviderWithDeadlinesTimeouts(p agentlib.Provider, requestTimeout, idleTimeout time.Duration) agentlib.Provider {
	if p == nil {
		return p
	}
	tp := &timeoutProvider{
		inner:          p,
		requestTimeout: requestTimeout,
		idleTimeout:    idleTimeout,
	}
	if _, ok := p.(agentlib.StreamingProvider); ok {
		return &streamingTimeoutProvider{timeoutProvider: tp}
	}
	return tp
}

// timeoutProvider enforces DefaultProviderRequestTimeout on Chat and forwards
// the agent library's optional metadata interfaces to the inner provider.
type timeoutProvider struct {
	inner          agentlib.Provider
	requestTimeout time.Duration
	idleTimeout    time.Duration
}

func (p *timeoutProvider) Chat(ctx context.Context, messages []agentlib.Message, tools []agentlib.ToolDef, opts agentlib.Options) (agentlib.Response, error) {
	if p.requestTimeout <= 0 {
		return p.inner.Chat(ctx, messages, tools, opts)
	}
	cctx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	resp, err := p.inner.Chat(cctx, messages, tools, opts)
	if err != nil && ctx.Err() == nil && cctx.Err() == context.DeadlineExceeded {
		return resp, fmt.Errorf("%w: wall-clock %s", ErrProviderRequestTimeout, p.requestTimeout)
	}
	return resp, err
}

// SessionStartMetadata forwards to the inner provider when implemented so
// agentlib.Run populates session.start telemetry with the real provider name
// and model rather than "unknown".
func (p *timeoutProvider) SessionStartMetadata() (string, string) {
	if mp, ok := p.inner.(interface {
		SessionStartMetadata() (string, string)
	}); ok {
		return mp.SessionStartMetadata()
	}
	return "", ""
}

// ChatStartMetadata forwards to the inner provider when implemented so chat
// span telemetry (provider_system, server_address, server_port) keeps flowing.
func (p *timeoutProvider) ChatStartMetadata() (string, string, int) {
	if mp, ok := p.inner.(interface {
		ChatStartMetadata() (string, string, int)
	}); ok {
		return mp.ChatStartMetadata()
	}
	return "", "", 0
}

// RoutingReport forwards to the inner provider when implemented so routing
// wrappers (e.g. backend-pool rotation) continue to report selected provider
// and failover counts through the deadline wrapper.
func (p *timeoutProvider) RoutingReport() agentlib.RoutingReport {
	if rr, ok := p.inner.(agentlib.RoutingReporter); ok {
		return rr.RoutingReport()
	}
	return agentlib.RoutingReport{}
}

// streamingTimeoutProvider adds ChatStream with an idle-read timer on top of
// timeoutProvider.
type streamingTimeoutProvider struct {
	*timeoutProvider
}

func (p *streamingTimeoutProvider) ChatStream(ctx context.Context, messages []agentlib.Message, tools []agentlib.ToolDef, opts agentlib.Options) (<-chan agentlib.StreamDelta, error) {
	sp, ok := p.inner.(agentlib.StreamingProvider)
	if !ok {
		return nil, fmt.Errorf("provider does not support streaming")
	}

	requestTimeout := p.requestTimeout
	idleTimeout := p.idleTimeout

	// Derive the per-stream context. It may already be unbounded if
	// requestTimeout is zero, in which case we only apply the idle-read cap.
	var (
		cctx   context.Context
		cancel context.CancelFunc
	)
	if requestTimeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, requestTimeout)
	} else {
		cctx, cancel = context.WithCancel(ctx)
	}

	ch, err := sp.ChatStream(cctx, messages, tools, opts)
	if err != nil {
		cancel()
		if ctx.Err() == nil && cctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%w: wall-clock %s", ErrProviderRequestTimeout, requestTimeout)
		}
		return nil, err
	}

	out := make(chan agentlib.StreamDelta, 1)
	go func() {
		defer cancel()
		defer close(out)

		var idle *time.Timer
		var idleC <-chan time.Time
		if idleTimeout > 0 {
			idle = time.NewTimer(idleTimeout)
			idleC = idle.C
			defer idle.Stop()
		}

		// emitted makes sure we only synthesize one error delta per stream.
		var emitted atomic.Bool
		emit := func(delta agentlib.StreamDelta) bool {
			select {
			case out <- delta:
				return true
			case <-ctx.Done():
				return false
			}
		}

		for {
			select {
			case delta, ok := <-ch:
				if !ok {
					return
				}
				if idle != nil {
					if !idle.Stop() {
						select {
						case <-idle.C:
						default:
						}
					}
					idle.Reset(idleTimeout)
				}
				if !emit(delta) {
					return
				}
				if delta.Done || delta.Err != nil {
					return
				}
			case <-idleC:
				if !emitted.CompareAndSwap(false, true) {
					return
				}
				cancel() // hard-abort the underlying HTTP read
				emit(agentlib.StreamDelta{
					Err: fmt.Errorf("%w: idle-read %s", ErrProviderRequestTimeout, idleTimeout),
				})
				// Drain the inner channel so the producer goroutine in the
				// underlying SDK returns, but only briefly — we already
				// signaled downstream.
				drainDeadline := time.NewTimer(2 * time.Second)
				defer drainDeadline.Stop()
				for {
					select {
					case _, ok := <-ch:
						if !ok {
							return
						}
					case <-drainDeadline.C:
						return
					}
				}
			case <-cctx.Done():
				// Distinguish wrapper-triggered wall-clock timeout from a
				// caller ctx cancel. When ctx is alive but cctx is dead by
				// DeadlineExceeded, we fired our own request-level cap.
				if ctx.Err() == nil && cctx.Err() == context.DeadlineExceeded {
					if !emitted.CompareAndSwap(false, true) {
						return
					}
					emit(agentlib.StreamDelta{
						Err: fmt.Errorf("%w: wall-clock %s", ErrProviderRequestTimeout, requestTimeout),
					})
				}
				// Either way, drain briefly then exit.
				drainDeadline := time.NewTimer(2 * time.Second)
				defer drainDeadline.Stop()
				for {
					select {
					case _, ok := <-ch:
						if !ok {
							return
						}
					case <-drainDeadline.C:
						return
					}
				}
			}
		}
	}()

	return out, nil
}

var (
	_ agentlib.Provider          = (*timeoutProvider)(nil)
	_ agentlib.Provider          = (*streamingTimeoutProvider)(nil)
	_ agentlib.StreamingProvider = (*streamingTimeoutProvider)(nil)
)
