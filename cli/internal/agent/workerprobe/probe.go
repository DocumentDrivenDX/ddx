// Package workerprobe implements the ADR-022 rev 5 worker-side server probe
// goroutine + best-effort event mirror.
//
// The worker is autonomous: it executes beads regardless of whether a server
// is reachable. In parallel, this Probe periodically checks
// ~/.local/share/ddx/server.addr (XDG-compliant) for a reachable server. On
// NotConnected→Connected transition the probe POSTs a thin identity envelope
// to /api/workers/register, stashes the issued worker_id, and starts mirroring
// per-attempt events to /api/workers/{id}/event best-effort. Buffered events
// from the NotConnected period are flushed via /api/workers/{id}/backfill.
//
// All HTTP calls are best-effort: failures are logged and ignored. The bead's
// local event log is the authoritative copy; the server-side derived view is
// observability only.
package workerprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// DefaultBufferCap is the ring-buffer size for events emitted while
// NotConnected. Per ADR-022 rev 5: 200 events; oldest dropped silently;
// HadDroppedBackfill flag on overflow.
const DefaultBufferCap = 200

// DefaultBaseInterval is the steady-state probe cadence (30s ± jitter).
const DefaultBaseInterval = 30 * time.Second

// DefaultMaxInterval caps the probe cadence under backoff (5min).
const DefaultMaxInterval = 5 * time.Minute

// DefaultJitterPct controls ± jitter applied to BaseInterval.
const DefaultJitterPct = 0.20

// DefaultBackoffThreshold is the consecutive-failure count after which the
// probe enters backoff (interval lengthens to MaxInterval).
const DefaultBackoffThreshold = 5

// Identity is the thin envelope POSTed on /api/workers/register.
type Identity struct {
	ProjectRoot  string    `json:"project_root"`
	Harness      string    `json:"harness"`
	Model        string    `json:"model,omitempty"`
	ExecutorPID  int       `json:"executor_pid"`
	ExecutorHost string    `json:"executor_host"`
	StartedAt    time.Time `json:"started_at"`
}

// Event mirrors a bead-event the worker would write to its local event log.
type Event struct {
	BeadID    string          `json:"bead_id"`
	AttemptID string          `json:"attempt_id"`
	Kind      string          `json:"kind"`
	Body      json.RawMessage `json:"body,omitempty"`
}

// Config tunes the probe cadence, jitter, backoff, and buffer capacity. Zero
// values mean "use the package default".
type Config struct {
	BaseInterval     time.Duration
	MaxInterval      time.Duration
	JitterPct        float64
	BackoffThreshold int
	BufferCap        int

	// AddrFunc returns the current server URL (scheme://host:port) or "" when
	// no server is registered. Default reads server.addr via the production
	// XDG path.
	AddrFunc func() string

	// HTTPClient is used for register/event/backfill POSTs. Default is a
	// short-timeout client.
	HTTPClient *http.Client

	// Now overrides time.Now (testing).
	Now func() time.Time

	// Rand overrides the jitter source (testing).
	Rand *rand.Rand

	// Logger receives best-effort failure messages (default: discarded).
	Logger io.Writer
}

func (c *Config) applyDefaults() {
	if c.BaseInterval == 0 {
		c.BaseInterval = DefaultBaseInterval
	}
	if c.MaxInterval == 0 {
		c.MaxInterval = DefaultMaxInterval
	}
	if c.JitterPct == 0 {
		c.JitterPct = DefaultJitterPct
	}
	if c.BackoffThreshold == 0 {
		c.BackoffThreshold = DefaultBackoffThreshold
	}
	if c.BufferCap == 0 {
		c.BufferCap = DefaultBufferCap
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if c.Logger == nil {
		c.Logger = io.Discard
	}
}

// Probe is the worker-side server probe + event mirror. Constructed via New;
// driven by Start; events fed via RecordEvent. Stop releases the goroutine.
type Probe struct {
	ident Identity
	cfg   Config

	mu       sync.Mutex
	state    state
	workerID string

	// buf is the NotConnected ring buffer. Events emitted while Connected
	// bypass the buffer (POSTed inline best-effort).
	buf     []Event
	dropped bool

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	// Test hooks: when non-nil, called after each probe iteration.
	OnProbe func(connected bool, interval time.Duration)
}

type state int

const (
	stateNotConnected state = iota
	stateConnected
)

// New constructs a Probe with the given identity. Config is copied; nil
// fields are filled with package defaults.
func New(ident Identity, cfg Config) *Probe {
	cfg.applyDefaults()
	return &Probe{
		ident:  ident,
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		buf:    make([]Event, 0, cfg.BufferCap),
	}
}

// Start launches the probe goroutine. The first probe fires immediately
// (no initial sleep), per ADR-022 §Probe + freshness state model. The
// goroutine returns when ctx is canceled or Stop is called.
func (p *Probe) Start(ctx context.Context) {
	go p.run(ctx)
}

// Stop signals the goroutine to exit and waits for it.
func (p *Probe) Stop() {
	p.stopOnce.Do(func() { close(p.stopCh) })
	<-p.doneCh
}

// Connected reports whether the probe currently sees the server as reachable.
func (p *Probe) Connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state == stateConnected
}

// WorkerID returns the most recent worker_id issued by the server, or "" if
// the worker is not currently registered.
func (p *Probe) WorkerID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workerID
}

// HadDroppedBackfill reports whether the ring buffer has overflowed since
// the last successful backfill.
func (p *Probe) HadDroppedBackfill() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dropped
}

// BufferLen returns the current ring-buffer length (test/diagnostic).
func (p *Probe) BufferLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buf)
}

// RecordEvent feeds a bead-event into the probe. If Connected, the event is
// POSTed to the server best-effort and not buffered. If NotConnected, the
// event lands in the ring buffer for backfill on next reconnect. Never
// blocks the caller on network I/O — POSTs are dispatched in a goroutine.
func (p *Probe) RecordEvent(ev Event) {
	p.mu.Lock()
	connected := p.state == stateConnected
	workerID := p.workerID
	addr := ""
	if connected && p.cfg.AddrFunc != nil {
		addr = p.cfg.AddrFunc()
	}
	if !connected || workerID == "" || addr == "" {
		p.appendBufferLocked(ev)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	go p.postEvent(addr, workerID, ev)
}

func (p *Probe) appendBufferLocked(ev Event) {
	if len(p.buf) >= p.cfg.BufferCap {
		// Drop oldest; record overflow.
		copy(p.buf, p.buf[1:])
		p.buf[len(p.buf)-1] = ev
		p.dropped = true
		return
	}
	p.buf = append(p.buf, ev)
}

func (p *Probe) run(ctx context.Context) {
	defer close(p.doneCh)

	// Immediate first probe (no initial sleep).
	p.tick(ctx)

	consecutiveFails := 0
	for {
		// Decide next interval: backoff after threshold consecutive failures,
		// otherwise jittered base.
		interval := p.cfg.BaseInterval
		if consecutiveFails >= p.cfg.BackoffThreshold {
			interval = p.cfg.MaxInterval
		}
		interval = applyJitter(interval, p.cfg.JitterPct, p.cfg.Rand)

		if p.OnProbe != nil {
			p.OnProbe(p.Connected(), interval)
		}

		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-p.stopCh:
			t.Stop()
			return
		case <-t.C:
		}

		ok := p.tick(ctx)
		if ok {
			consecutiveFails = 0
		} else {
			consecutiveFails++
		}
	}
}

// tick performs one probe cycle and returns true on a successful reach
// (server replied to register or to a register that was already in place).
func (p *Probe) tick(ctx context.Context) bool {
	addr := ""
	if p.cfg.AddrFunc != nil {
		addr = p.cfg.AddrFunc()
	}
	if addr == "" {
		// No server configured. Drive Connected→NotConnected if we used to
		// be connected; otherwise no-op.
		p.markDisconnected("no_addr")
		return false
	}

	// Already connected: cheap reachability check by hitting register again
	// is overkill; we just trust the connection and let event POSTs surface
	// failures via the failure-counter (markDisconnected is called from
	// postEvent when the POST fails). This keeps probe traffic minimal.
	if p.Connected() {
		return true
	}

	// NotConnected: attempt to register.
	workerID, err := p.postRegister(ctx, addr)
	if err != nil {
		fmt.Fprintf(p.cfg.Logger, "workerprobe: register failed: %v\n", err)
		return false
	}
	p.markConnected(workerID)

	// Drain the ring buffer via backfill (best-effort).
	p.flushBackfill(ctx, addr, workerID)
	return true
}

func (p *Probe) markConnected(workerID string) {
	p.mu.Lock()
	p.state = stateConnected
	p.workerID = workerID
	p.mu.Unlock()
}

func (p *Probe) markDisconnected(reason string) {
	p.mu.Lock()
	if p.state == stateConnected {
		fmt.Fprintf(p.cfg.Logger, "workerprobe: disconnect (%s)\n", reason)
	}
	p.state = stateNotConnected
	p.workerID = ""
	p.mu.Unlock()
}

func (p *Probe) flushBackfill(ctx context.Context, addr, workerID string) {
	p.mu.Lock()
	if len(p.buf) == 0 && !p.dropped {
		p.mu.Unlock()
		return
	}
	events := make([]Event, len(p.buf))
	copy(events, p.buf)
	dropped := p.dropped
	p.mu.Unlock()

	if err := p.postBackfill(ctx, addr, workerID, events, dropped); err != nil {
		fmt.Fprintf(p.cfg.Logger, "workerprobe: backfill failed: %v\n", err)
		return
	}

	// Success: clear the ring buffer.
	p.mu.Lock()
	p.buf = p.buf[:0]
	p.dropped = false
	p.mu.Unlock()
}

// postRegister POSTs the identity envelope and returns the issued worker_id.
func (p *Probe) postRegister(ctx context.Context, addr string) (string, error) {
	body, err := json.Marshal(p.ident)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr+"/api/workers/register", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("register: status=%d", resp.StatusCode)
	}
	var out struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.WorkerID == "" {
		return "", fmt.Errorf("register: empty worker_id")
	}
	return out.WorkerID, nil
}

// postEvent best-effort mirrors a single event. On failure, marks the probe
// NotConnected so the next probe tick re-registers (and the event re-enters
// the ring buffer the next time RecordEvent is called).
func (p *Probe) postEvent(addr, workerID string, ev Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		fmt.Fprintf(p.cfg.Logger, "workerprobe: event marshal failed: %v\n", err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, addr+"/api/workers/"+workerID+"/event", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(p.cfg.Logger, "workerprobe: event request failed: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		fmt.Fprintf(p.cfg.Logger, "workerprobe: event POST failed: %v\n", err)
		// Re-buffer the event so a future backfill catches it; flip state.
		p.mu.Lock()
		p.appendBufferLocked(ev)
		p.mu.Unlock()
		p.markDisconnected("event_post_error")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNoContent:
		// Mirrored.
	case http.StatusGone:
		// Server forgot us (restarted between register and event). Mark
		// disconnected; next probe will re-register.
		p.mu.Lock()
		p.appendBufferLocked(ev)
		p.mu.Unlock()
		p.markDisconnected("event_unknown_worker")
	default:
		fmt.Fprintf(p.cfg.Logger, "workerprobe: event status=%d\n", resp.StatusCode)
		p.mu.Lock()
		p.appendBufferLocked(ev)
		p.mu.Unlock()
	}
}

// postBackfill replays the NotConnected ring buffer.
func (p *Probe) postBackfill(ctx context.Context, addr, workerID string, events []Event, dropped bool) error {
	body, err := json.Marshal(struct {
		Events  []Event `json:"events"`
		Dropped bool    `json:"dropped,omitempty"`
	}{Events: events, Dropped: dropped})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr+"/api/workers/"+workerID+"/backfill", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("backfill: status=%d", resp.StatusCode)
	}
	return nil
}

// TeeJSONL returns an io.Writer that forwards every write to base AND parses
// each newline-terminated JSON envelope into a workerprobe.Event fed to the
// probe. Envelope shape matches writeLoopEvent (session_id/type/ts/data with
// data.bead_id and data.attempt_id when present). Malformed lines are passed
// through unchanged but skipped for mirroring.
func TeeJSONL(base io.Writer, p *Probe) io.Writer {
	return &teeWriter{base: base, probe: p}
}

type teeWriter struct {
	base  io.Writer
	probe *Probe
	buf   []byte
}

func (t *teeWriter) Write(b []byte) (int, error) {
	n, err := t.base.Write(b)
	// Accumulate; emit one Event per newline-terminated chunk.
	t.buf = append(t.buf, b...)
	for {
		i := indexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		line := t.buf[:i]
		t.buf = t.buf[i+1:]
		t.dispatch(line)
	}
	return n, err
}

func (t *teeWriter) dispatch(line []byte) {
	if len(line) == 0 {
		return
	}
	var env struct {
		Type string `json:"type"`
		Data struct {
			BeadID    string `json:"bead_id"`
			AttemptID string `json:"attempt_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(line, &env); err != nil || env.Type == "" {
		return
	}
	// Body carries the full envelope so the server log preserves structured
	// detail (loop.start config, bead.result outcome, etc.).
	body := json.RawMessage(append([]byte(nil), line...))
	t.probe.RecordEvent(Event{
		BeadID:    env.Data.BeadID,
		AttemptID: env.Data.AttemptID,
		Kind:      env.Type,
		Body:      body,
	})
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// applyJitter returns base scaled by a uniform random factor in
// [1-pct, 1+pct]. pct is clamped to [0, 1).
func applyJitter(base time.Duration, pct float64, r *rand.Rand) time.Duration {
	if pct <= 0 {
		return base
	}
	if pct >= 1 {
		pct = 0.99
	}
	// Uniform in [-pct, +pct].
	delta := (r.Float64()*2 - 1) * pct
	scaled := time.Duration(float64(base) * (1 + delta))
	if scaled < time.Millisecond {
		scaled = time.Millisecond
	}
	return scaled
}
