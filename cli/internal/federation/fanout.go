package federation

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultFanOutMaxConcurrency caps the number of in-flight per-spoke requests.
const DefaultFanOutMaxConcurrency = 8

// DefaultFanOutPerNodeTimeout is the default per-node deadline for one fan-out call.
const DefaultFanOutPerNodeTimeout = 5 * time.Second

// SkipReason is a short, machine-readable code describing why a spoke was
// skipped before any HTTP call was issued.
type SkipReason string

const (
	// SkipReasonIncompatibleVersion indicates the version handshake against the
	// hub's own version+schema rejected this spoke (e.g. major mismatch,
	// schema mismatch, unparseable ddx_version, …). The Handshake reason is
	// included in the logged message.
	SkipReasonIncompatibleVersion SkipReason = "incompatible_version"
	// SkipReasonMissingCapability indicates the spoke does not advertise one of
	// the capabilities required for this query.
	SkipReasonMissingCapability SkipReason = "missing_capability"
	// SkipReasonInvalidURL indicates the spoke's persisted URL did not parse.
	SkipReasonInvalidURL SkipReason = "invalid_url"
)

// NodeOutcomeKind describes how a single spoke contributed (or did not
// contribute) to a fan-out call. The hub uses this to update per-spoke status
// distinctly: offline (immediate transport failure on this fan-out) vs stale
// (heartbeat-overdue badge from the registry) vs ok.
type NodeOutcomeKind string

const (
	// OutcomeOK means the spoke returned a response (any HTTP 2xx).
	OutcomeOK NodeOutcomeKind = "ok"
	// OutcomeOffline means the transport itself failed: connection refused,
	// DNS error, TLS handshake failed, mid-response disconnect, etc. The hub
	// flips the spoke's registry status to StatusOffline on this outcome.
	OutcomeOffline NodeOutcomeKind = "offline"
	// OutcomeTimeout means the per-node timeout fired before a response was
	// fully received. Distinct from offline so callers can keep the spoke's
	// last-known status (likely stale) rather than promote it to offline.
	OutcomeTimeout NodeOutcomeKind = "timeout"
	// OutcomeHTTPError means the spoke responded with a non-2xx status. The
	// transport worked, so we do not flip to offline; the body is captured.
	OutcomeHTTPError NodeOutcomeKind = "http_error"
	// OutcomeSkipped means the spoke was filtered out before fan-out; see
	// NodeResult.SkipReason.
	OutcomeSkipped NodeOutcomeKind = "skipped"
)

// FanOutRequest is the GraphQL payload to send to every active spoke.
type FanOutRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
	// RequiredCapabilities, if non-empty, filters out spokes that do not
	// advertise every named capability in their SpokeRecord.Capabilities.
	RequiredCapabilities []string `json:"-"`
}

// NodeResult is a single spoke's contribution to a fan-out call.
type NodeResult struct {
	NodeID string
	URL    string
	// Status, when non-empty, is the registry status the hub should record for
	// this spoke as a result of this fan-out (StatusOffline on transport
	// failure). Empty means "do not change registry status".
	Status SpokeStatus
	// Outcome classifies how this spoke contributed.
	Outcome NodeOutcomeKind
	// Response is the raw GraphQL response body for OutcomeOK. The hub merges
	// it into the federated response shape. Empty otherwise.
	Response json.RawMessage
	// HTTPStatus is the HTTP status code seen, or 0 on transport failure.
	HTTPStatus int
	// Err is the underlying error for non-OK outcomes.
	Err error
	// SkipReason is set when Outcome == OutcomeSkipped.
	SkipReason SkipReason
	// SkipDetail is a free-form human message accompanying SkipReason.
	SkipDetail string
	// Duration is the wall time spent on this spoke's call.
	Duration time.Duration
}

// FanOutResult is the merged outcome of a fan-out call.
type FanOutResult struct {
	// Responses holds one entry per spoke that returned 2xx, keyed by node_id.
	// The value is the raw GraphQL response body.
	Responses map[string]json.RawMessage
	// Errors holds one entry per spoke that produced any non-OK outcome
	// (offline, timeout, http_error). Skipped spokes are NOT in this map —
	// they appear in Skipped instead. Keyed by node_id.
	Errors map[string]error
	// Skipped holds one entry per spoke filtered out before fan-out, keyed by
	// node_id, with the SkipReason as the value.
	Skipped map[string]SkipReason
	// StatusUpdates lists the (node_id → new status) changes the hub should
	// apply to its registry as a side effect of this fan-out. Currently only
	// transport-level failures (StatusOffline) are emitted here; heartbeat-
	// driven status (stale) is left to the registry's own time-based reconcile.
	StatusUpdates map[string]SpokeStatus
	// Nodes is the per-node detail (every spoke considered, including skipped).
	// Order matches the input registry order for deterministic test output.
	Nodes []NodeResult
}

// FanOutClient executes fan-out GraphQL queries across a federation registry.
type FanOutClient struct {
	// HTTPClient is used for spoke /graphql calls. Nil → a default client that
	// accepts self-signed TLS certs (matches the spoke server's auto-cert).
	HTTPClient *http.Client
	// MaxConcurrency caps concurrent in-flight requests. ≤0 → default.
	MaxConcurrency int
	// PerNodeTimeout is the per-spoke deadline. ≤0 → default.
	PerNodeTimeout time.Duration
	// HubDDxVersion / HubSchemaVersion drive the version handshake gate.
	// Empty → version filter is skipped (test convenience).
	HubDDxVersion    string
	HubSchemaVersion string
	// Logger receives info/warn lines; nil → log.Printf.
	Logger func(format string, args ...any)
	// Now is the time source used for Duration measurement. Nil → time.Now.
	Now func() time.Time
}

// NewFanOutClient builds a FanOutClient with defaults filled in.
func NewFanOutClient() *FanOutClient {
	return &FanOutClient{
		HTTPClient: &http.Client{
			// Per-request deadlines drive cancellation; this is just a safety
			// net for misbehaving servers that never close the connection.
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		},
		MaxConcurrency: DefaultFanOutMaxConcurrency,
		PerNodeTimeout: DefaultFanOutPerNodeTimeout,
	}
}

func (c *FanOutClient) logf(format string, args ...any) {
	if c.Logger != nil {
		c.Logger(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (c *FanOutClient) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *FanOutClient) maxConcurrency() int {
	if c.MaxConcurrency <= 0 {
		return DefaultFanOutMaxConcurrency
	}
	return c.MaxConcurrency
}

func (c *FanOutClient) perNodeTimeout() time.Duration {
	if c.PerNodeTimeout <= 0 {
		return DefaultFanOutPerNodeTimeout
	}
	return c.PerNodeTimeout
}

func (c *FanOutClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// Execute fans out req across spokes. Returns a merged FanOutResult; never
// returns a non-nil error for per-spoke failures — those are surfaced through
// FanOutResult.Errors. A non-nil error is reserved for misuse (e.g. nil req).
//
// The set of spokes considered is the input slice. Callers typically pass the
// hub registry's spokes filtered by Status == active|stale (offline/degraded
// handling depends on policy and is left to the caller). Skipped spokes still
// appear in the per-node Nodes detail with OutcomeSkipped + SkipReason.
//
// Cancellation: if ctx is canceled, in-flight goroutines abort their HTTP
// calls and return OutcomeOffline (with ctx.Err() as cause); callers that want
// to distinguish caller-cancellation from spoke failure should check ctx.Err()
// after Execute returns.
func (c *FanOutClient) Execute(ctx context.Context, spokes []SpokeRecord, req *FanOutRequest) (*FanOutResult, error) {
	if req == nil {
		return nil, errors.New("federation: fanout request is nil")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, errors.New("federation: fanout request query is empty")
	}

	body, err := json.Marshal(struct {
		Query         string         `json:"query"`
		OperationName string         `json:"operationName,omitempty"`
		Variables     map[string]any `json:"variables,omitempty"`
	}{req.Query, req.OperationName, req.Variables})
	if err != nil {
		return nil, fmt.Errorf("federation: marshal fanout query: %w", err)
	}

	results := make([]NodeResult, len(spokes))
	sem := make(chan struct{}, c.maxConcurrency())
	var wg sync.WaitGroup

	for i := range spokes {
		i := i
		s := spokes[i]
		// Pre-flight filters: version, capability, URL.
		if skip, reason, detail := c.preflightSkip(s, req.RequiredCapabilities); skip {
			c.logf("federation fanout: skip node_id=%q reason=%s detail=%q", s.NodeID, reason, detail)
			results[i] = NodeResult{
				NodeID:     s.NodeID,
				URL:        s.URL,
				Outcome:    OutcomeSkipped,
				SkipReason: reason,
				SkipDetail: detail,
			}
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = NodeResult{
					NodeID:  s.NodeID,
					URL:     s.URL,
					Outcome: OutcomeOffline,
					Status:  StatusOffline,
					Err:     ctx.Err(),
				}
				return
			}
			defer func() { <-sem }()
			results[i] = c.callOne(ctx, s, body)
		}()
	}
	wg.Wait()

	out := &FanOutResult{
		Responses:     map[string]json.RawMessage{},
		Errors:        map[string]error{},
		Skipped:       map[string]SkipReason{},
		StatusUpdates: map[string]SpokeStatus{},
		Nodes:         results,
	}
	for _, r := range results {
		switch r.Outcome {
		case OutcomeOK:
			out.Responses[r.NodeID] = r.Response
		case OutcomeSkipped:
			out.Skipped[r.NodeID] = r.SkipReason
		default:
			if r.Err != nil {
				out.Errors[r.NodeID] = r.Err
			} else {
				out.Errors[r.NodeID] = fmt.Errorf("fanout: %s", r.Outcome)
			}
		}
		if r.Status != "" {
			out.StatusUpdates[r.NodeID] = r.Status
		}
	}
	return out, nil
}

// preflightSkip returns (true, reason, detail) when the spoke must be skipped
// before any HTTP call is issued.
func (c *FanOutClient) preflightSkip(s SpokeRecord, requiredCaps []string) (bool, SkipReason, string) {
	if c.HubDDxVersion != "" && c.HubSchemaVersion != "" {
		hs := Handshake(c.HubDDxVersion, c.HubSchemaVersion, s.DDxVersion, s.SchemaVersion)
		if !hs.Accept {
			return true, SkipReasonIncompatibleVersion, hs.Reason
		}
	}
	if len(requiredCaps) > 0 {
		have := map[string]struct{}{}
		for _, c := range s.Capabilities {
			// Skip the synthetic identity-fingerprint capability.
			if strings.HasPrefix(c, "@identity:") {
				continue
			}
			have[c] = struct{}{}
		}
		for _, want := range requiredCaps {
			if _, ok := have[want]; !ok {
				return true, SkipReasonMissingCapability, want
			}
		}
	}
	if strings.TrimSpace(s.URL) == "" {
		return true, SkipReasonInvalidURL, "empty url"
	}
	if _, err := url.Parse(s.URL); err != nil {
		return true, SkipReasonInvalidURL, err.Error()
	}
	return false, "", ""
}

// callOne issues the GraphQL POST against one spoke under per-node timeout.
func (c *FanOutClient) callOne(ctx context.Context, s SpokeRecord, body []byte) NodeResult {
	start := c.now()
	cctx, cancel := context.WithTimeout(ctx, c.perNodeTimeout())
	defer cancel()

	endpoint := strings.TrimRight(s.URL, "/") + "/graphql"
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return NodeResult{
			NodeID:   s.NodeID,
			URL:      s.URL,
			Outcome:  OutcomeOffline,
			Status:   StatusOffline,
			Err:      err,
			Duration: c.now().Sub(start),
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		// Distinguish per-node timeout (deadline exceeded on cctx but parent
		// ctx still live) from caller cancellation and from generic transport
		// failure. Per-node timeout → keep last-known status; transport
		// failure → flip to offline.
		if cctx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			return NodeResult{
				NodeID:   s.NodeID,
				URL:      s.URL,
				Outcome:  OutcomeTimeout,
				Err:      fmt.Errorf("per-node timeout after %s: %w", c.perNodeTimeout(), err),
				Duration: c.now().Sub(start),
			}
		}
		return NodeResult{
			NodeID:   s.NodeID,
			URL:      s.URL,
			Outcome:  OutcomeOffline,
			Status:   StatusOffline,
			Err:      err,
			Duration: c.now().Sub(start),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	buf, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		// Mid-response disconnect — transport-level failure → offline.
		return NodeResult{
			NodeID:     s.NodeID,
			URL:        s.URL,
			Outcome:    OutcomeOffline,
			Status:     StatusOffline,
			HTTPStatus: resp.StatusCode,
			Err:        fmt.Errorf("read body: %w", readErr),
			Duration:   c.now().Sub(start),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(buf)
		if len(snippet) > 256 {
			snippet = snippet[:256] + "…"
		}
		return NodeResult{
			NodeID:     s.NodeID,
			URL:        s.URL,
			Outcome:    OutcomeHTTPError,
			HTTPStatus: resp.StatusCode,
			Err:        fmt.Errorf("spoke returned HTTP %d: %s", resp.StatusCode, snippet),
			Duration:   c.now().Sub(start),
		}
	}

	return NodeResult{
		NodeID:     s.NodeID,
		URL:        s.URL,
		Outcome:    OutcomeOK,
		HTTPStatus: resp.StatusCode,
		Response:   json.RawMessage(buf),
		Duration:   c.now().Sub(start),
	}
}
