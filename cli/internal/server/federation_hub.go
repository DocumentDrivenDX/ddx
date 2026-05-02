package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// HubDefaultDDxVersion is the ddx_version the hub advertises when validating
// spoke handshakes. It is exposed as a field so tests and operators can pin
// the hub's view of "current ddx version" without rebuilding the binary.
const HubDefaultDDxVersion = "0.1.0"

// federationHub holds the per-server state for hub-mode federation: the
// in-memory registry, the disk path used to persist it, the warning logger
// (for plain-HTTP accepted registrations), and configuration knobs.
type federationHub struct {
	mu             sync.Mutex
	registry       *federation.FederationRegistry
	statePath      string // empty disables disk persistence
	allowPlainHTTP bool
	hubDDxVersion  string
	hubSchemaVer   string
	warnLog        func(format string, args ...any)
	warnings       []string // captured plain-HTTP warnings (test introspection)
	conflictsLog   []string // captured 409 conflict log lines (test introspection)
	now            func() time.Time
}

// EnableHubMode mounts the /api/federation/* routes on this server, switches
// the hub-mode flag on, and (best effort) loads the registry from disk so a
// hub restart can rebuild state from a prior session.
//
// Must be called before ListenAndServe / ListenAndServeTLS. Calling it more
// than once is a no-op after the first call.
//
// allowPlainHTTP relaxes the default ts-net/loopback peer policy on the
// federation routes — non-trusted peers are accepted, but each accepted
// registration emits a WARN log entry. This is the operator opt-out for
// federations spanning networks where ts-net is not viable.
func (s *Server) EnableHubMode(allowPlainHTTP bool) {
	if s.hub != nil {
		return
	}
	statePath, err := federation.DefaultStatePath()
	if err != nil {
		statePath = ""
	}
	var reg *federation.FederationRegistry
	if statePath != "" {
		if loaded, lerr := federation.LoadStateFrom(statePath); lerr == nil {
			reg = loaded
		}
	}
	if reg == nil {
		reg = federation.NewRegistry()
	}
	s.hub = &federationHub{
		registry:       reg,
		statePath:      statePath,
		allowPlainHTTP: allowPlainHTTP,
		hubDDxVersion:  HubDefaultDDxVersion,
		hubSchemaVer:   federation.CurrentSchemaVersion,
		warnLog:        func(format string, args ...any) { log.Printf("WARN: "+format, args...) },
		now:            time.Now,
	}
	s.HubMode = true

	gate := func(h http.HandlerFunc) http.HandlerFunc { return s.requireFederationTrusted(h) }
	s.route("POST /api/federation/register", gate(s.handleFederationRegister))
	s.route("POST /api/federation/heartbeat", gate(s.handleFederationHeartbeat))
	s.route("GET /api/federation/spokes", gate(s.handleFederationListSpokes))
	s.route("DELETE /api/federation/spokes/{id}", gate(s.handleFederationDeregister))
}

// HubFederationRegistrySnapshot returns a copy of the current registry, for
// tests and operator introspection.
func (s *Server) HubFederationRegistrySnapshot() *federation.FederationRegistry {
	if s.hub == nil {
		return nil
	}
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	out := federation.NewRegistry()
	out.SchemaVersion = s.hub.registry.SchemaVersion
	out.Spokes = append(out.Spokes, s.hub.registry.Spokes...)
	return out
}

// HubFederationWarnings returns plain-HTTP-accepted-registration WARN messages
// captured since the server started. Test-only helper.
func (s *Server) HubFederationWarnings() []string {
	if s.hub == nil {
		return nil
	}
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return append([]string(nil), s.hub.warnings...)
}

// HubFederationConflicts returns 409 conflict log messages captured since the
// server started. Test-only helper.
func (s *Server) HubFederationConflicts() []string {
	if s.hub == nil {
		return nil
	}
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return append([]string(nil), s.hub.conflictsLog...)
}

// requireFederationTrusted enforces the federation-specific peer policy:
// loopback / ts-net peers are always accepted; non-trusted peers are accepted
// only when --federation-allow-plain-http is set. Distinct from
// requireTrusted so the AC-strict default doesn't depend on a global flag.
func (s *Server) requireFederationTrusted(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isTrusted(r) {
			next(w, r)
			return
		}
		if s.hub != nil && s.hub.allowPlainHTTP {
			next(w, r)
			return
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: ts-net or loopback peer required for /api/federation/* (use --federation-allow-plain-http to opt out)"})
	}
}

// federationRegisterRequest is the spoke→hub registration / re-registration
// payload. Identity = (NodeID, IdentityFingerprint). NodeID is the stable
// federation key; IdentityFingerprint distinguishes a re-registration of the
// same node from a duplicate-name conflict (different machine claiming an
// already-registered node_id).
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

// federationRegisterResponse is sent on a successful (200/201) registration.
type federationRegisterResponse struct {
	NodeID string                 `json:"node_id"`
	Status federation.SpokeStatus `json:"status"`
	Reason string                 `json:"reason"`
	Spoke  federation.SpokeRecord `json:"spoke"`
}

// identityFingerprintKey is the SpokeRecord.Capabilities-adjacent slot we
// stash the fingerprint in. To keep the on-disk schema stable (B14.2 froze
// SpokeRecord shape), we encode it as a synthetic capability prefixed with
// "@identity:". Future schema versions can hoist it to a first-class field.
const identityFingerprintPrefix = "@identity:"

func setIdentityFingerprint(rec *federation.SpokeRecord, fp string) {
	cleaned := rec.Capabilities[:0]
	for _, c := range rec.Capabilities {
		if !strings.HasPrefix(c, identityFingerprintPrefix) {
			cleaned = append(cleaned, c)
		}
	}
	rec.Capabilities = cleaned
	if fp != "" {
		rec.Capabilities = append(rec.Capabilities, identityFingerprintPrefix+fp)
	}
}

func getIdentityFingerprint(rec federation.SpokeRecord) string {
	for _, c := range rec.Capabilities {
		if strings.HasPrefix(c, identityFingerprintPrefix) {
			return strings.TrimPrefix(c, identityFingerprintPrefix)
		}
	}
	return ""
}

func (s *Server) handleFederationRegister(w http.ResponseWriter, r *http.Request) {
	var req federationRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	if req.NodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_id required"})
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url required"})
		return
	}

	hs := federation.Handshake(s.hub.hubDDxVersion, s.hub.hubSchemaVer, req.DDxVersion, req.SchemaVersion)
	if !hs.Accept {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "version handshake failed",
			"reason": hs.Reason,
		})
		return
	}

	now := s.hub.now().UTC()

	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()

	if existing := s.hub.registry.FindSpoke(req.NodeID); existing != nil {
		existingFP := getIdentityFingerprint(*existing)
		// Identity conflict: a different machine claims an already-registered
		// node_id. Reject with 409 and log.
		if existingFP != "" && req.IdentityFingerprint != "" && existingFP != req.IdentityFingerprint {
			msg := fmt.Sprintf("federation: duplicate node_id %q from different identity (existing=%s, attempted=%s)",
				req.NodeID, existingFP, req.IdentityFingerprint)
			s.hub.conflictsLog = append(s.hub.conflictsLog, msg)
			s.hub.warnLog("%s", msg)
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":  "node_id already registered to a different identity",
				"reason": "duplicate_node_id",
			})
			return
		}
	}

	rec := federation.SpokeRecord{
		NodeID:        req.NodeID,
		Name:          req.Name,
		URL:           req.URL,
		DDxVersion:    req.DDxVersion,
		SchemaVersion: req.SchemaVersion,
		Capabilities:  append([]string(nil), req.Capabilities...),
		RegisteredAt:  now,
		LastHeartbeat: time.Time{},
		Status:        hs.Status,
	}
	setIdentityFingerprint(&rec, req.IdentityFingerprint)
	if err := s.hub.registry.UpsertSpoke(rec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.persistFederationLocked()

	// Emit WARN if this registration was accepted via the plain-HTTP opt-out.
	if s.hub.allowPlainHTTP && !isTrusted(r) {
		msg := fmt.Sprintf("federation: accepted plain-HTTP registration node_id=%q url=%q from remote=%s",
			req.NodeID, req.URL, r.RemoteAddr)
		s.hub.warnings = append(s.hub.warnings, msg)
		s.hub.warnLog("%s", msg)
	}

	writeJSON(w, http.StatusOK, federationRegisterResponse{
		NodeID: rec.NodeID,
		Status: rec.Status,
		Reason: hs.Reason,
		Spoke:  rec,
	})
}

type federationHeartbeatRequest struct {
	NodeID string `json:"node_id"`
}

func (s *Server) handleFederationHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req federationHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	if req.NodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_id required"})
		return
	}

	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()

	rec := s.hub.registry.FindSpoke(req.NodeID)
	if rec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "spoke not registered"})
		return
	}
	now := s.hub.now().UTC()
	rec.LastHeartbeat = now
	// Heartbeat clears registered-on-first-seen and offline; preserves
	// degraded (set by the version handshake at registration time).
	if rec.Status == federation.StatusRegistered || rec.Status == federation.StatusOffline || rec.Status == federation.StatusStale {
		rec.Status = federation.StatusActive
	}
	s.persistFederationLocked()

	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":        rec.NodeID,
		"status":         rec.Status,
		"last_heartbeat": rec.LastHeartbeat,
	})
}

func (s *Server) handleFederationListSpokes(w http.ResponseWriter, _ *http.Request) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	out := make([]federation.SpokeRecord, 0, len(s.hub.registry.Spokes))
	out = append(out, s.hub.registry.Spokes...)
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": s.hub.registry.SchemaVersion,
		"spokes":         out,
	})
}

func (s *Server) handleFederationDeregister(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if !s.hub.registry.RemoveSpoke(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "spoke not found"})
		return
	}
	s.persistFederationLocked()
	writeJSON(w, http.StatusOK, map[string]string{"node_id": id, "status": "deregistered"})
}

// persistFederationLocked saves the registry to disk if a state path is set.
// Caller must hold s.hub.mu.
func (s *Server) persistFederationLocked() {
	if s.hub.statePath == "" {
		return
	}
	if err := federation.SaveStateTo(s.hub.statePath, s.hub.registry); err != nil {
		s.hub.warnLog("federation: persist state: %v", err)
	}
}
