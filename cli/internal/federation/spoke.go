package federation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultHeartbeatInterval is the nominal heartbeat cadence (ADR-007: 30s).
const DefaultHeartbeatInterval = 30 * time.Second

// DefaultHeartbeatJitterFraction is applied to the interval as ±jitter so
// independent spokes do not synchronize their beats against one hub.
const DefaultHeartbeatJitterFraction = 0.25

// DefaultDeregisterTimeout caps the best-effort deregister request on shutdown.
const DefaultDeregisterTimeout = 2 * time.Second

// spokeStateFileName is the on-disk name for spoke-side state, used to detect
// URL changes across restarts so the spoke can re-register with a new URL.
const spokeStateFileName = "spoke-state.json"

// SpokeState is the per-spoke persistent state. It records the URL last
// advertised to the hub and the stable identity_fingerprint generated on
// first start, both so URL changes can be detected across restarts and so
// re-registration can prove identity to the hub.
type SpokeState struct {
	NodeID              string `json:"node_id"`
	IdentityFingerprint string `json:"identity_fingerprint"`
	LastURL             string `json:"last_url,omitempty"`
}

// DefaultSpokeStatePath returns ~/.local/share/ddx/spoke-state.json (XDG aware).
func DefaultSpokeStatePath() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ddx", spokeStateFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("federation: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "ddx", spokeStateFileName), nil
}

// LoadSpokeState reads spoke state from path. Missing file → zero value.
func LoadSpokeState(path string) (*SpokeState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &SpokeState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("federation: read spoke state: %w", err)
	}
	var st SpokeState
	if err := json.Unmarshal(data, &st); err != nil {
		// Corrupt file — start fresh; the caller will overwrite.
		return &SpokeState{}, nil
	}
	return &st, nil
}

// SaveSpokeState writes spoke state to path atomically (tmpfile+rename).
func SaveSpokeState(path string, st *SpokeState) error {
	if st == nil {
		return fmt.Errorf("federation: nil spoke state")
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("federation: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".spoke-state-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, fileMode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// generateIdentityFingerprint returns a random 16-byte hex string. The
// fingerprint is generated once per node and persisted in spoke-state so a
// re-registration of the same node_id from the same machine is recognized
// as such (rather than rejected as a duplicate-identity conflict).
func generateIdentityFingerprint() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fall back to a hostname-derived value; collisions here are
		// uninteresting because the hub's duplicate-id rejection only fires
		// on a *different* fingerprint claiming the same node_id.
		host, _ := os.Hostname()
		return "fp-" + host
	}
	return "fp-" + hex.EncodeToString(buf[:])
}

// SpokeConfig configures a Spoke lifecycle agent. URL is the spoke's own
// publicly-reachable URL (advertised to the hub); HubURL is the hub base URL
// (e.g. "https://hub.example:7743").
type SpokeConfig struct {
	NodeID               string
	Name                 string
	URL                  string
	HubURL               string
	DDxVersion           string
	SchemaVersion        string
	GraphQLSchemaVersion string
	Capabilities         []string

	// HeartbeatInterval defaults to DefaultHeartbeatInterval. Pass a small
	// value (e.g. 50ms) in tests.
	HeartbeatInterval time.Duration
	// HeartbeatJitterFraction is the ± jitter as a fraction of the interval
	// (e.g. 0.25 for ±25%). Defaults to DefaultHeartbeatJitterFraction.
	HeartbeatJitterFraction float64
	// DeregisterTimeout caps the best-effort deregister on shutdown.
	DeregisterTimeout time.Duration

	// StatePath is the disk path for spoke-state.json. Empty → DefaultSpokeStatePath.
	StatePath string
	// HTTPClient is used for hub requests. Nil → default client that accepts
	// self-signed certs (matches the server's auto-generated cert).
	HTTPClient *http.Client
	// Logger is used for warn/info; nil → log.Printf.
	Logger func(format string, args ...any)
}

// Spoke is the spoke-side lifecycle agent: register on Start, jittered
// heartbeat in the background, deregister best-effort on Shutdown.
type Spoke struct {
	cfg                 SpokeConfig
	identityFingerprint string

	mu              sync.Mutex
	statePath       string
	registered      bool
	stopped         chan struct{}
	stopOnce        sync.Once
	registeredCount int // test introspection
	heartbeatCount  int // test introspection
	urlChanged      bool
	urlChangedFrom  string
	hbErr           error
	logf            func(format string, args ...any)
}

// NewSpoke constructs a Spoke with defaults filled in. The spoke is not
// started until Start is called.
func NewSpoke(cfg SpokeConfig) (*Spoke, error) {
	if strings.TrimSpace(cfg.NodeID) == "" {
		return nil, fmt.Errorf("federation: spoke NodeID required")
	}
	if strings.TrimSpace(cfg.HubURL) == "" {
		return nil, fmt.Errorf("federation: spoke HubURL required")
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("federation: spoke URL required")
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.HeartbeatJitterFraction < 0 {
		cfg.HeartbeatJitterFraction = 0
	}
	if cfg.HeartbeatJitterFraction == 0 {
		cfg.HeartbeatJitterFraction = DefaultHeartbeatJitterFraction
	}
	if cfg.DeregisterTimeout <= 0 {
		cfg.DeregisterTimeout = DefaultDeregisterTimeout
	}
	if cfg.HTTPClient == nil {
		// Self-signed certs are the default for ddx-server, so the spoke
		// must skip cert verification by default. Operators wanting stricter
		// validation can pass a custom HTTPClient.
		cfg.HTTPClient = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		}
	}

	statePath := cfg.StatePath
	if statePath == "" {
		p, err := DefaultSpokeStatePath()
		if err != nil {
			return nil, err
		}
		statePath = p
	}

	logf := cfg.Logger
	if logf == nil {
		logf = func(format string, args ...any) { log.Printf(format, args...) }
	}

	st, err := LoadSpokeState(statePath)
	if err != nil {
		return nil, err
	}

	// Detect URL change between restarts.
	var urlChanged bool
	var prevURL string
	if st.NodeID == cfg.NodeID && st.LastURL != "" && st.LastURL != cfg.URL {
		urlChanged = true
		prevURL = st.LastURL
	}

	// Reuse fingerprint if it's for the same node_id; otherwise generate one.
	fp := st.IdentityFingerprint
	if fp == "" || st.NodeID != cfg.NodeID {
		fp = generateIdentityFingerprint()
	}

	s := &Spoke{
		cfg:                 cfg,
		identityFingerprint: fp,
		statePath:           statePath,
		stopped:             make(chan struct{}),
		urlChanged:          urlChanged,
		urlChangedFrom:      prevURL,
		logf:                logf,
	}
	return s, nil
}

// IdentityFingerprint returns the stable identity fingerprint persisted for
// this spoke. Useful for tests and operator diagnostics.
func (s *Spoke) IdentityFingerprint() string { return s.identityFingerprint }

// URLChanged reports whether the spoke detected a URL change versus the
// last-known persisted state at startup time.
func (s *Spoke) URLChanged() bool { return s.urlChanged }

// PreviousURL returns the URL recorded in spoke-state on disk before this
// start (empty if none). Distinct from URLChanged so callers can log either.
func (s *Spoke) PreviousURL() string { return s.urlChangedFrom }

// HeartbeatCount returns the number of heartbeats sent (test introspection).
func (s *Spoke) HeartbeatCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.heartbeatCount
}

// RegisteredCount returns the number of register calls made (test introspection).
func (s *Spoke) RegisteredCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registeredCount
}

// LastHeartbeatError returns the most recent heartbeat error or nil.
func (s *Spoke) LastHeartbeatError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hbErr
}

// Start performs the initial register-on-start and launches the jittered
// heartbeat goroutine. It returns the register error (if any); a successful
// Start means the hub accepted the registration.
//
// Start blocks until the initial registration completes (or fails). The
// heartbeat goroutine continues until Shutdown is called or ctx is canceled.
func (s *Spoke) Start(ctx context.Context) error {
	if err := s.register(ctx); err != nil {
		return err
	}
	// Persist current URL so the next start can detect a URL change.
	_ = SaveSpokeState(s.statePath, &SpokeState{
		NodeID:              s.cfg.NodeID,
		IdentityFingerprint: s.identityFingerprint,
		LastURL:             s.cfg.URL,
	})
	go s.heartbeatLoop(ctx)
	return nil
}

// Shutdown stops the heartbeat loop and sends a best-effort deregister.
// Safe to call multiple times. Always returns nil — deregister failures are
// logged but not surfaced (the hub may legitimately be down).
func (s *Spoke) Shutdown(ctx context.Context) error {
	s.stopOnce.Do(func() { close(s.stopped) })

	// Best-effort deregister with a bounded timeout so a dead hub does not
	// stall shutdown.
	dctx, cancel := context.WithTimeout(ctx, s.cfg.DeregisterTimeout)
	defer cancel()
	if err := s.deregister(dctx); err != nil {
		s.logf("federation: deregister best-effort failed: %v", err)
	}
	return nil
}

// register POSTs /api/federation/register to the hub.
func (s *Spoke) register(ctx context.Context) error {
	body := federationRegisterRequest{
		NodeID:               s.cfg.NodeID,
		IdentityFingerprint:  s.identityFingerprint,
		Name:                 s.cfg.Name,
		URL:                  s.cfg.URL,
		DDxVersion:           s.cfg.DDxVersion,
		SchemaVersion:        s.cfg.SchemaVersion,
		GraphQLSchemaVersion: s.cfg.GraphQLSchemaVersion,
		Capabilities:         append([]string(nil), s.cfg.Capabilities...),
	}
	resp, err := s.postJSON(ctx, "/api/federation/register", body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("federation register: hub returned %d: %s", resp.StatusCode, string(buf))
	}
	s.mu.Lock()
	s.registered = true
	s.registeredCount++
	s.mu.Unlock()
	return nil
}

// deregister DELETEs /api/federation/spokes/<id>.
func (s *Spoke) deregister(ctx context.Context) error {
	u, err := s.hubURL("/api/federation/spokes/" + url.PathEscape(s.cfg.NodeID))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	// 404 is fine — nothing to deregister.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("federation deregister: hub returned %d", resp.StatusCode)
	}
	return nil
}

// heartbeat POSTs /api/federation/heartbeat.
func (s *Spoke) heartbeat(ctx context.Context) error {
	body := map[string]string{"node_id": s.cfg.NodeID}
	resp, err := s.postJSON(ctx, "/api/federation/heartbeat", body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		// Hub forgot us — re-register.
		s.logf("federation: hub returned 404 on heartbeat; re-registering")
		return s.register(ctx)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("federation heartbeat: hub returned %d", resp.StatusCode)
	}
	return nil
}

// heartbeatLoop runs until ctx is canceled or Shutdown is called. Sleeps for
// HeartbeatInterval ± jitter between beats.
func (s *Spoke) heartbeatLoop(ctx context.Context) {
	for {
		d := s.nextHeartbeatDelay()
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case <-time.After(d):
		}
		if err := s.heartbeat(ctx); err != nil {
			s.mu.Lock()
			s.hbErr = err
			s.mu.Unlock()
			s.logf("federation: heartbeat failed: %v", err)
		} else {
			s.mu.Lock()
			s.hbErr = nil
			s.heartbeatCount++
			s.mu.Unlock()
		}
	}
}

// NextHeartbeatDelay returns the next sleep duration the heartbeat loop will
// pick: HeartbeatInterval ± random jitter bounded by HeartbeatJitterFraction.
// Exposed so tests can verify jitter bounds without sleeping for 30s.
func (s *Spoke) NextHeartbeatDelay() time.Duration { return s.nextHeartbeatDelay() }

// nextHeartbeatDelay returns interval ± random jitter in [0, jitter*interval].
func (s *Spoke) nextHeartbeatDelay() time.Duration {
	base := s.cfg.HeartbeatInterval
	jf := s.cfg.HeartbeatJitterFraction
	if jf <= 0 {
		return base
	}
	// Random offset in (-jf*base, +jf*base).
	maxOffset := int64(float64(base) * jf)
	if maxOffset <= 0 {
		return base
	}
	n, err := rand.Int(rand.Reader, big.NewInt(2*maxOffset+1))
	if err != nil {
		return base
	}
	offset := n.Int64() - maxOffset
	d := base + time.Duration(offset)
	if d < 0 {
		d = base
	}
	return d
}

func (s *Spoke) hubURL(path string) (string, error) {
	base := strings.TrimRight(s.cfg.HubURL, "/")
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *Spoke) postJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	u, err := s.hubURL(path)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return s.cfg.HTTPClient.Do(req)
}

// federationRegisterRequest mirrors the hub-side request shape. Defined here
// to keep the federation package self-contained — the hub side
// (cli/internal/server/federation_hub.go) declares its own equivalent struct
// against the same wire contract.
type federationRegisterRequest struct {
	NodeID               string   `json:"node_id"`
	IdentityFingerprint  string   `json:"identity_fingerprint,omitempty"`
	Name                 string   `json:"name"`
	URL                  string   `json:"url"`
	DDxVersion           string   `json:"ddx_version"`
	SchemaVersion        string   `json:"schema_version"`
	GraphQLSchemaVersion string   `json:"graphql_schema_version,omitempty"`
	Capabilities         []string `json:"capabilities,omitempty"`
}
