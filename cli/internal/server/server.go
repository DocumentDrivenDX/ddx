package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/metric"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/DocumentDrivenDX/ddx/internal/processmetrics"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
	"tailscale.com/tsnet"
)

// serverPromptCapBytes caps the size of inline prompt bodies accepted on
// server-side ingress (POST /api/agent/run). FEAT-022 §9 / Stage D2. It is
// a package-level variable rather than a literal so tests can lower the
// cap without writing multi-MB request bodies. Production callers must not
// mutate it.
var serverPromptCapBytes = evidence.DefaultMaxPromptBytes

// serverBuildSHA returns the build's VCS revision for inclusion on
// operator-prompt audit events. Falls back to "unknown" when build info is
// unavailable (e.g. `go test` without -buildvcs or development builds).
func serverBuildSHA() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value
		}
	}
	return "unknown"
}

// serverInlineCapBytes caps inline content bodies returned on egress paths
// — MCP tool responses and REST JSON responses that carry library-doc,
// execution-bundle, diff, session, or persona text. FEAT-022 §10 / §11.
// Stage E1. Package-level variable so tests can lower the cap; production
// callers must not mutate it.
var serverInlineCapBytes = evidence.DefaultMaxInlinedFileBytes

// mcpText constructs a text-typed mcpContent whose Text body is bounded by
// serverInlineCapBytes via evidence.ClampOutput. The Truncated and
// OriginalBytes fields are always populated (false/len(text) when the body
// fits within the cap). This is the single mcpContent{Type:"text"} literal
// site in this package — every other caller must route through this helper
// so static analysis (TestEvidencePrimitiveUsage) can guarantee no
// unbounded text egress on the MCP surface. FEAT-022 §10 / Stage E1.
func mcpText(text string) mcpContent {
	clamped, truncated, originalBytes := evidence.ClampOutput(text, serverInlineCapBytes)
	// evidence:allow-unbounded reason="single mcpContent{Type:\"text\"} literal site for the package; Text is sourced from evidence.ClampOutput above (FEAT-022 §10 Stage E1)"
	return mcpContent{Type: "text", Text: clamped, Truncated: truncated, OriginalBytes: originalBytes}
}

// beadHubCloser is the minimal interface the Server requires from its bead
// lifecycle hub. *bead.WatcherHub satisfies this interface.
type beadHubCloser interface {
	SubscribeLifecycle(projectID string) (<-chan bead.LifecycleEvent, func())
	Close()
}

// Server is the DDx HTTP server exposing REST and MCP endpoints.
type Server struct {
	Addr        string
	WorkingDir  string
	TsnetConfig *config.TsnetConfig
	mux         *http.ServeMux
	startTime   time.Time
	workers     *WorkerManager
	beadHub     beadHubCloser
	state       *ServerState

	// csrfTokens issues and validates CSRF tokens for write mutations on
	// the GraphQL endpoint (Story 15 / operator-prompts). The token is
	// generated once at server construction and exposed to clients via
	// GET /api/csrf-token (localhost-gated).
	csrfTokens *ddxgraphql.StaticCSRFTokenStore
	// operatorPromptIdempotency deduplicates operatorPromptSubmit calls by
	// idempotency key within a 24-hour window. Process-local; survives
	// only for the lifetime of the server process.
	operatorPromptIdempotency *ddxgraphql.MemoryIdempotencyCache
	// operatorPromptAutoApproveAllowlist is the per-project allowlist of
	// localhost identity actors permitted to auto-approve their own
	// operator-prompt submissions and to invoke operatorPromptApprove. The
	// locked Story 15 decision restricts approval to configured-localhost
	// identities; ts-net peers are never eligible. Seeded from
	// DDX_OPERATOR_PROMPT_ALLOWLIST (comma-separated) at server construction
	// and may be overridden by tests via SetOperatorPromptAutoApproveAllowlist.
	operatorPromptAutoApproveAllowlist []string

	routePatterns []string // every pattern registered via route(); used by gate tests

	// HubMode is true once EnableHubMode has been called; the federation
	// HTTP routes are mounted only in that case (B14.3 AC: "Routes mounted
	// only when --hub-mode").
	HubMode bool
	hub     *federationHub

	// SpokeMode is true once EnableSpokeMode has been called. Used together
	// with HubMode to compute node.federation_role:
	//   neither   → "standalone"
	//   hub only  → "hub"
	//   spoke only→ "spoke"
	//   both      → "hub_spoke"
	SpokeMode bool
	spoke     *federationSpoke

	// gqlHandler is the singleton gqlgen HTTP handler. LAYER 2 of the
	// GraphQL multi-project fix (ddx-055e8d32) builds this once at first
	// request and reuses it across both /graphql and the scoped
	// /api/projects/{project}/graphql route — per-request reconstruction
	// (LAYER 1) is no longer needed because the resolver reads WorkingDir
	// from the request context.
	gqlOnce    sync.Once
	gqlHandler http.Handler

	// workerIngest is the in-memory derived view of worker reports per
	// ADR-022 rev 5 §Worker-server interface. Workers POST register/event/
	// backfill best-effort; the registry is non-authoritative.
	workerIngest *workerIngestRegistry

	// reportedWorkers adapts workerIngest into the GraphQL reportedWorkers
	// resolver. Held on the Server (rather than constructed lazily) so the
	// ADR-022 step 5c integration test can swap the freshness clock and so
	// every /graphql request shares one instance.
	reportedWorkers *reportedWorkersAdapter
}

// New creates a new DDx server bound to addr, serving data from workingDir.
func New(addr, workingDir string) *Server {
	nodeName := resolveNodeName()
	stateDir := serverAddrDir() // XDG-standard user-level dir, one per node
	state := loadServerState(stateDir, nodeName)
	state.workingDir = workingDir

	workers := NewWorkerManager(workingDir)
	workers.ReconcileStaleWorkers()
	beadHub := bead.NewWatcherHub(250 * time.Millisecond)
	csrfStore, err := ddxgraphql.NewStaticCSRFTokenStore()
	if err != nil {
		// CSRF token generation should never fail outside of a broken
		// crypto/rand. Fall back to a deterministic stub that rejects
		// every request rather than failing open.
		csrfStore = ddxgraphql.NewStaticCSRFTokenStoreWithToken("")
	}
	s := &Server{
		Addr:                               addr,
		WorkingDir:                         workingDir,
		mux:                                http.NewServeMux(),
		startTime:                          time.Now().UTC(),
		workers:                            workers,
		beadHub:                            beadHub,
		state:                              state,
		csrfTokens:                         csrfStore,
		operatorPromptIdempotency:          ddxgraphql.NewMemoryIdempotencyCache(),
		operatorPromptAutoApproveAllowlist: parseOperatorPromptAllowlistEnv(os.Getenv("DDX_OPERATOR_PROMPT_ALLOWLIST")),
		workerIngest:                       newWorkerIngestRegistry(workingDir),
	}
	s.reportedWorkers = newReportedWorkersAdapter(s.workerIngest)
	state.coordinatorReg = workers.LandCoordinators

	// Register the server's own project immediately.
	state.RegisterProject(workingDir)
	_ = state.save()

	s.routes()
	return s
}

// parseOperatorPromptAllowlistEnv parses a comma-separated identity list
// (DDX_OPERATOR_PROMPT_ALLOWLIST) into a normalized allowlist. Empty entries
// are skipped; whitespace is trimmed. The literal sentinel "localhost"
// matches any localhost-kind actor.
func parseOperatorPromptAllowlistEnv(raw string) []string {
	if raw = strings.TrimSpace(raw); raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// SetOperatorPromptAutoApproveAllowlist replaces the in-memory allowlist
// used by operatorPromptSubmit (with autoApprove=true) and operatorPromptApprove.
// Tests use this to install a deterministic allowlist without juggling env vars.
func (s *Server) SetOperatorPromptAutoApproveAllowlist(allowlist []string) {
	s.operatorPromptAutoApproveAllowlist = append([]string(nil), allowlist...)
}

// resolveNodeName returns DDX_NODE_NAME env, or the system hostname.
func resolveNodeName() string {
	if n := os.Getenv("DDX_NODE_NAME"); n != "" {
		return n
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

// Handler returns the server's HTTP handler (useful for testing).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// State exposes the server's persistent state so adjacent packages (notably
// the perf harness) can register additional projects or query snapshots
// without reaching into unexported internals. The returned value MUST NOT be
// mutated concurrently with the server — it is the same pointer the server
// itself uses.
func (s *Server) State() *ServerState {
	return s.state
}

// RegisterProject adds (or refreshes) a project entry on the server's state
// and persists the updated state file. Returns the stored entry.
func (s *Server) RegisterProject(path string) ProjectEntry {
	entry := s.state.RegisterProject(path)
	_ = s.state.save()
	return entry
}

// Shutdown stops the server's background services: closes the bead lifecycle
// hub and stops all land coordinators. Returns the first error encountered.
// Both operations are idempotent and safe to call on an idle server.
func (s *Server) Shutdown() error {
	// Best-effort spoke deregister so the hub registry does not show this
	// node as stale after a graceful shutdown. Errors are swallowed inside
	// ShutdownSpoke (the hub may legitimately be down).
	if s.spoke != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.ShutdownSpoke(ctx)
		cancel()
	}
	s.beadHub.Close()
	s.workers.LandCoordinators.StopAll()
	return nil
}

// ListenAndServe starts the server. If TsnetConfig.Enabled is true, a parallel
// Tailscale ts-net listener is started alongside the standard localhost listener.
func (s *Server) ListenAndServe() error {
	release, err := acquireSingletonLock()
	if err != nil {
		return err
	}
	defer release()
	s.installSingletonReleaseOnSignal(release)
	s.writeAddrFile("http")
	if s.TsnetConfig != nil && s.TsnetConfig.Enabled {
		errCh := make(chan error, 2)

		// Standard localhost listener
		go func() {
			errCh <- http.ListenAndServe(s.Addr, s.mux)
		}()

		// ts-net listener
		go func() {
			errCh <- s.listenTsnet()
		}()

		return <-errCh
	}
	return http.ListenAndServe(s.Addr, s.mux)
}

// ListenAndServeTLS starts the server with TLS. If certFile and keyFile are
// empty, a self-signed certificate is auto-generated and cached under
// workingDir/.ddx/server/tls/.
func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	release, err := acquireSingletonLock()
	if err != nil {
		return err
	}
	defer release()
	s.installSingletonReleaseOnSignal(release)
	if certFile == "" || keyFile == "" {
		var err error
		tlsDir := filepath.Join(s.WorkingDir, ".ddx", "server", "tls")
		certFile, keyFile, err = ensureSelfSignedCert(tlsDir)
		if err != nil {
			return fmt.Errorf("generating self-signed cert: %w", err)
		}
	}
	s.writeAddrFile("https")
	if s.TsnetConfig != nil && s.TsnetConfig.Enabled {
		errCh := make(chan error, 2)
		go func() {
			errCh <- http.ListenAndServeTLS(s.Addr, certFile, keyFile, s.mux)
		}()
		go func() {
			errCh <- s.listenTsnet()
		}()
		return <-errCh
	}
	return http.ListenAndServeTLS(s.Addr, certFile, keyFile, s.mux)
}

// writeAddrFile writes the server's address to a user-level file so CLI
// clients can auto-discover it without configuration.
func (s *Server) writeAddrFile(scheme string) {
	type addrFile struct {
		Node      string    `json:"node"`
		NodeID    string    `json:"node_id"`
		URL       string    `json:"url"`
		PID       int       `json:"pid"`
		StartedAt time.Time `json:"started_at"`
	}
	af := addrFile{
		Node:      s.state.Node.Name,
		NodeID:    s.state.Node.ID,
		URL:       fmt.Sprintf("%s://%s", scheme, s.Addr),
		PID:       os.Getpid(),
		StartedAt: s.startTime,
	}
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return
	}
	dir := serverAddrDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "server.addr"), data, 0600)
}

// serverAddrDir returns the user-level directory for the server address file.
// Follows XDG_DATA_HOME if set, else ~/.local/share/ddx.
func serverAddrDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ddx")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", "ddx")
	}
	return filepath.Join(home, ".local", "share", "ddx")
}

// ReadServerAddr reads the last-written server address from the user-level
// addr file. Returns "" if none is present, or if the recorded pid is not
// alive (in which case a warning is emitted and the stale file is best-effort
// cleared so callers fall back to the default URL).
func ReadServerAddr() string {
	type addrFile struct {
		URL string `json:"url"`
		PID int    `json:"pid"`
	}
	path := filepath.Join(serverAddrDir(), "server.addr")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var af addrFile
	if err := json.Unmarshal(data, &af); err != nil {
		return ""
	}
	if af.PID > 0 && !processAlive(af.PID) {
		fmt.Fprintf(addrFallbackWarnWriter,
			"warning: server.addr points to a dead pid %d; falling back to default 127.0.0.1:7743\n",
			af.PID)
		_ = os.Remove(path)
		return ""
	}
	return af.URL
}

// installSingletonReleaseOnSignal arranges for the singleton lock to be
// released on SIGTERM/SIGINT so that the next ddx-server can start without
// having to wait for stale-lock detection. The handler exits the process
// with a conventional non-zero status; defers in ListenAndServe(TLS) do not
// run, but the lock release fires explicitly here.
func (s *Server) installSingletonReleaseOnSignal(release func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		release()
		os.Exit(130)
	}()
}

// ensureSelfSignedCert returns paths to a self-signed cert/key in dir,
// generating them if they don't already exist.
func ensureSelfSignedCert(dir string) (certFile, keyFile string, err error) {
	if err = os.MkdirAll(dir, 0700); err != nil {
		return
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	// Re-use existing pair if both files are present and cert is still valid.
	if _, e1 := os.Stat(certFile); e1 == nil {
		if _, e2 := os.Stat(keyFile); e2 == nil {
			if pair, e3 := tls.LoadX509KeyPair(certFile, keyFile); e3 == nil {
				leaf, e4 := x509.ParseCertificate(pair.Certificate[0])
				if e4 == nil && time.Now().Before(leaf.NotAfter.Add(-24*time.Hour)) {
					return certFile, keyFile, nil
				}
			}
		}
	}

	// Generate new key + cert.
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		err = e
		return
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "ddx-server"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")},
		DNSNames:     []string{"localhost"},
	}
	certDER, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if e != nil {
		err = e
		return
	}

	cf, e := os.OpenFile(certFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		err = e
		return
	}
	_ = pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	_ = cf.Close()

	kf, e := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		err = e
		return
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	_ = pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	_ = kf.Close()

	return certFile, keyFile, nil
}

// listenTsnet starts a Tailscale ts-net TLS listener and serves the same mux.
func (s *Server) listenTsnet() error {
	tc := s.TsnetConfig
	hostname := tc.Hostname
	if hostname == "" {
		hostname = "ddx"
	}
	stateDir := tc.StateDir
	if stateDir == "" {
		stateDir = filepath.Join(s.WorkingDir, ".ddx", "tsnet")
	}

	ts := &tsnet.Server{
		Hostname: hostname,
		Dir:      stateDir,
		AuthKey:  tc.AuthKey,
	}
	defer func() { _ = ts.Close() }()

	ln, err := ts.ListenTLS("tcp", ":443")
	if err != nil {
		return fmt.Errorf("tsnet listen: %w", err)
	}
	defer func() { _ = ln.Close() }()

	fmt.Printf("DDx ts-net listener active: https://%s\n", hostname)
	return http.Serve(ln, tsnetMiddleware(ts, s.mux))
}

// tsnetMiddleware wraps the handler to inject Tailscale identity context and
// mark requests as coming from the tailnet (for dispatch endpoint access).
func tsnetMiddleware(ts *tsnet.Server, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lc, err := ts.LocalClient()
		if err == nil {
			who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
			if err == nil && who != nil {
				// Log caller identity for audit
				login := who.UserProfile.LoginName
				node := who.Node.ComputedName
				r.Header.Set("X-Tailscale-User", login)
				r.Header.Set("X-Tailscale-Node", node)
				// Mark request as trusted tailnet peer so dispatch endpoints allow it
				r = r.WithContext(context.WithValue(r.Context(), tsnetTrustedKey{}, true))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// tsnetTrustedKey is the context key for ts-net trusted connections.
type tsnetTrustedKey struct{}

// isTrusted returns true if the request is from localhost or a ts-net peer.
func isTrusted(r *http.Request) bool {
	if v, ok := r.Context().Value(tsnetTrustedKey{}).(bool); ok && v {
		return true
	}
	return isLocalhost(r)
}

// isLocalhostAddr parses a RemoteAddr and checks for loopback.
func isLocalhostAddr(addr string) bool {
	host := addr
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

// requireTrusted wraps a handler so non-loopback, non-tailnet requests are
// rejected with 403 before the handler executes.
func requireTrusted(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isTrusted(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: trusted connection required"})
			return
		}
		next(w, r)
	}
}

// projectContextKey is the context key under which the resolved ProjectEntry
// for a /api/projects/{project}/... request is stored.
type projectContextKey struct{}

// projectFromContext returns the ProjectEntry previously injected by
// projectScoped or singletonScoped middleware, if any.
func projectFromContext(ctx context.Context) (ProjectEntry, bool) {
	p, ok := ctx.Value(projectContextKey{}).(ProjectEntry)
	return p, ok
}

// withProjectContext returns a new context carrying p under projectContextKey.
func withProjectContext(ctx context.Context, p ProjectEntry) context.Context {
	return context.WithValue(ctx, projectContextKey{}, p)
}

// resolveProject returns the ProjectEntry matching key (by ID first, then path).
func (s *Server) resolveProject(key string) (ProjectEntry, bool) {
	if entry, ok := s.state.GetProjectByID(key); ok {
		return entry, true
	}
	return s.state.GetProjectByPath(key)
}

// mcpResolveWorkingDir resolves a project argument passed to a project-local
// MCP tool into a working directory. Semantics mirror the HTTP routing layer:
//
//   - project provided (ID or path): resolve via resolveProject; 400-equivalent
//     error result if not found.
//   - project omitted + exactly one project registered: auto-resolve (singleton
//     compat — the common single-project deployment).
//   - project omitted + >1 project registered: return a disambiguation error.
//   - project omitted + 0 projects registered: fall back to s.WorkingDir.
//
// The error mcpToolResult (if non-nil) should be returned directly to the MCP
// client; workingDir is empty when an error is returned.
func (s *Server) mcpResolveWorkingDir(project string) (string, *mcpToolResult) {
	if project != "" {
		entry, ok := s.resolveProject(project)
		if !ok {
			return "", &mcpToolResult{
				Content: []mcpContent{mcpText(fmt.Sprintf("project not found: %s", project))},
				IsError: true,
			}
		}
		return entry.Path, nil
	}
	projects := s.state.GetProjects()
	switch len(projects) {
	case 0:
		return s.WorkingDir, nil
	case 1:
		return projects[0].Path, nil
	default:
		return "", &mcpToolResult{
			Content: []mcpContent{mcpText("multiple projects registered; specify 'project' argument (id or path)")},
			IsError: true,
		}
	}
}

// projectScoped is middleware for /api/projects/{project}/... routes.
// It pulls {project} out of the path, resolves it to a registered ProjectEntry,
// and injects the entry into the request context. Requests whose project does
// not resolve get 404.
func (s *Server) projectScoped(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("project")
		entry, ok := s.resolveProject(key)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		next(w, r.WithContext(withProjectContext(r.Context(), entry)))
	}
}

// singletonScoped is middleware for legacy /api/... routes. When exactly one
// project is registered, it injects that project into the request context so
// legacy handlers behave identically to the scoped /api/projects/{id}/...
// equivalent. When zero or more than one project is registered the request
// passes through unchanged, preserving the historical multi-project aggregate
// behavior of list endpoints and the server's own WorkingDir default for
// single-item endpoints.
func (s *Server) singletonScoped(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects := s.state.GetProjects()
		if len(projects) == 1 {
			r = r.WithContext(withProjectContext(r.Context(), projects[0]))
		}
		next(w, r)
	}
}

// workingDirForRequest returns the filesystem root this request should operate
// against. Scoped routes get the resolved project's path; unscoped routes fall
// back to the server's own WorkingDir.
func (s *Server) workingDirForRequest(r *http.Request) string {
	if p, ok := projectFromContext(r.Context()); ok {
		return p.Path
	}
	return s.WorkingDir
}

// libraryPathForRequest returns the library path for the request's project.
func (s *Server) libraryPathForRequest(r *http.Request) string {
	return s.libraryPathFor(s.workingDirForRequest(r))
}

// beadStoreForRequest returns a bead store rooted at the request's project.
func (s *Server) beadStoreForRequest(r *http.Request) *bead.Store {
	return bead.NewStore(filepath.Join(s.workingDirForRequest(r), ".ddx"))
}

// buildDocGraphForRequest builds the docgraph from the request's project root.
func (s *Server) buildDocGraphForRequest(r *http.Request) (*docgraph.Graph, error) {
	return docgraph.BuildGraphWithConfig(s.workingDirForRequest(r))
}

// execStoreForRequest returns an exec store rooted at the request's project.
func (s *Server) execStoreForRequest(r *http.Request) *ddxexec.Store {
	return ddxexec.NewStore(s.workingDirForRequest(r))
}

// workerManagerForRequest returns the live WorkerManager when the request
// resolves to the server's own WorkingDir, and a read-only manager rooted at
// the request's project otherwise. Requests with no project context fall back
// to s.workers.
func (s *Server) workerManagerForRequest(r *http.Request) *WorkerManager {
	dir := s.workingDirForRequest(r)
	if dir == s.WorkingDir {
		return s.workers
	}
	return NewWorkerManager(dir)
}

// runsRedirectSunset is the deprecation date advertised in the Sunset header
// for the /sessions and /executions routes that were merged into Runs.
// RFC 8594: Sunset is an HTTP-date.
const runsRedirectSunset = "Wed, 31 Dec 2026 00:00:00 GMT"

// runsRedirectHandler builds an http.HandlerFunc that 302-redirects to the
// URL produced by target(r), setting the Sunset header for client tooling
// that respects the deprecation signal.
func runsRedirectHandler(target func(r *http.Request) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Sunset", runsRedirectSunset)
		w.Header().Set("Deprecation", "true")
		http.Redirect(w, r, target(r), http.StatusFound)
	}
}

// route registers a handler and records the pattern for test introspection.
func (s *Server) route(pattern string, handler http.HandlerFunc) {
	s.routePatterns = append(s.routePatterns, pattern)
	s.mux.HandleFunc(pattern, handler)
}

func (s *Server) routes() {
	// Health — no trust gate.
	s.route("GET /api/health", s.handleHealth)
	s.route("GET /api/ready", s.handleReady)

	// Every other endpoint is gated on isTrusted() via the requireTrusted
	// middleware. If you add a new route, use trusted() so the gate test
	// (TestAllNonHealthHandlersGateOnIsTrusted) stays green.
	trusted := func(pattern string, handler http.HandlerFunc) {
		s.route(pattern, requireTrusted(handler))
	}
	// Legacy unscoped routes auto-resolve to the single registered project
	// (singleton compatibility). When more than one project is registered the
	// request passes through unchanged — list endpoints keep their
	// cross-project aggregation behavior, single-item endpoints keep their
	// s.WorkingDir default.
	legacy := func(pattern string, handler http.HandlerFunc) {
		trusted(pattern, s.singletonScoped(handler))
	}
	// Project-scoped routes extract {project} from the URL, resolve it, and
	// inject the ProjectEntry into the request context.
	scoped := func(pattern string, handler http.HandlerFunc) {
		trusted(pattern, s.projectScoped(handler))
	}

	// CSRF token (operator-prompt mutations) — gated to trusted callers so
	// the served HTML can fetch a per-session token to send via the
	// X-CSRF-Token header on operatorPromptSubmit/Approve/Cancel.
	trusted("GET /api/csrf-token", s.handleCSRFToken)

	// Node + project registry
	trusted("GET /api/node", s.handleGetNode)
	trusted("GET /api/projects", s.handleListProjects)
	trusted("POST /api/projects/register", s.handleRegisterProject)
	trusted("GET /api/projects/{project}/commits", s.handleProjectCommits)

	// Documents — legacy
	legacy("GET /api/documents", s.handleListDocuments)
	legacy("PUT /api/documents/{path...}", s.handleWriteDocument)
	legacy("GET /api/documents/{path...}", s.handleReadDocument)
	legacy("GET /api/search", s.handleSearch)
	trusted("GET /api/personas", s.handleListPersonas)
	trusted("GET /api/personas/{role}", s.handleResolvePersona)

	// Documents — project-scoped (FEAT-002: canonical)
	scoped("GET /api/projects/{project}/documents", s.handleListDocuments)
	scoped("PUT /api/projects/{project}/documents/{path...}", s.handleWriteDocument)
	scoped("GET /api/projects/{project}/documents/{path...}", s.handleReadDocument)
	scoped("GET /api/projects/{project}/search", s.handleSearch)

	// Artifacts — project-scoped (FEAT-008)
	scoped("GET /api/projects/{project}/artifact-content", s.handleArtifactContent)

	// Beads — legacy
	legacy("GET /api/beads", s.handleListBeads)
	legacy("GET /api/beads/ready", s.handleBeadsReady)
	legacy("GET /api/beads/blocked", s.handleBeadsBlocked)
	legacy("GET /api/beads/status", s.handleBeadsStatus)
	legacy("GET /api/beads/dep/tree/{id}", s.handleBeadDepTree)
	legacy("GET /api/beads/{id}", s.handleShowBead)
	legacy("GET /api/beads/{id}/evidence", s.handleBeadEvidence)
	legacy("GET /api/beads/{id}/cooldown", s.handleBeadCooldown)
	legacy("GET /api/beads/{id}/routing", s.handleBeadRouting)

	// Beads — project-scoped (FEAT-002: canonical)
	scoped("GET /api/projects/{project}/beads", s.handleListBeads)
	scoped("GET /api/projects/{project}/beads/ready", s.handleBeadsReady)
	scoped("GET /api/projects/{project}/beads/blocked", s.handleBeadsBlocked)
	scoped("GET /api/projects/{project}/beads/status", s.handleBeadsStatus)
	scoped("GET /api/projects/{project}/beads/dep/tree/{id}", s.handleBeadDepTree)
	scoped("GET /api/projects/{project}/beads/{id}", s.handleShowBead)
	scoped("GET /api/projects/{project}/beads/{id}/evidence", s.handleBeadEvidence)
	scoped("GET /api/projects/{project}/beads/{id}/cooldown", s.handleBeadCooldown)
	scoped("GET /api/projects/{project}/beads/{id}/routing", s.handleBeadRouting)

	// Doc graph — legacy
	legacy("GET /api/docs/graph", s.handleDocGraph)
	legacy("GET /api/docs/stale", s.handleDocStale)
	legacy("GET /api/docs/changed", s.handleDocChanged)
	legacy("GET /api/docs/{id}/deps", s.handleDocDeps)
	legacy("GET /api/docs/{id}/dependents", s.handleDocDependents)
	legacy("GET /api/docs/{id}/history", s.handleDocHistory)
	legacy("GET /api/docs/{id}/diff", s.handleDocDiff)
	legacy("PUT /api/docs/{id}", s.handleDocWrite)
	legacy("GET /api/docs/{id}", s.handleDocShow)

	// Doc graph — project-scoped (FEAT-002: canonical)
	scoped("GET /api/projects/{project}/docs/graph", s.handleDocGraph)
	scoped("GET /api/projects/{project}/docs/stale", s.handleDocStale)
	scoped("GET /api/projects/{project}/docs/changed", s.handleDocChanged)
	scoped("GET /api/projects/{project}/docs/{id}/deps", s.handleDocDeps)
	scoped("GET /api/projects/{project}/docs/{id}/dependents", s.handleDocDependents)
	scoped("GET /api/projects/{project}/docs/{id}/history", s.handleDocHistory)
	scoped("GET /api/projects/{project}/docs/{id}/diff", s.handleDocDiff)
	scoped("PUT /api/projects/{project}/docs/{id}", s.handleDocWrite)
	scoped("GET /api/projects/{project}/docs/{id}", s.handleDocShow)

	// Bead mutations — legacy
	legacy("POST /api/beads", s.handleCreateBead)
	legacy("PUT /api/beads/{id}", s.handleUpdateBead)
	legacy("POST /api/beads/{id}/claim", s.handleClaimBead)
	legacy("POST /api/beads/{id}/unclaim", s.handleUnclaimBead)
	legacy("POST /api/beads/{id}/reopen", s.handleReopenBead)
	legacy("POST /api/beads/{id}/cancel", s.handleCancelBead)
	legacy("POST /api/beads/{id}/deps", s.handleBeadDeps)

	// Bead mutations — project-scoped (FEAT-002: canonical)
	scoped("POST /api/projects/{project}/beads", s.handleCreateBead)
	scoped("PUT /api/projects/{project}/beads/{id}", s.handleUpdateBead)
	scoped("POST /api/projects/{project}/beads/{id}/claim", s.handleClaimBead)
	scoped("POST /api/projects/{project}/beads/{id}/unclaim", s.handleUnclaimBead)
	scoped("POST /api/projects/{project}/beads/{id}/reopen", s.handleReopenBead)
	scoped("POST /api/projects/{project}/beads/{id}/cancel", s.handleCancelBead)
	scoped("POST /api/projects/{project}/beads/{id}/deps", s.handleBeadDeps)

	// Agent model/catalog/capabilities — legacy + project-scoped (FEAT-006)
	legacy("GET /api/agent/models", s.handleAgentModels)
	legacy("GET /api/agent/catalog", s.handleAgentCatalog)
	legacy("GET /api/agent/capabilities", s.handleAgentCapabilities)
	scoped("GET /api/projects/{project}/agent/models", s.handleAgentModels)
	scoped("GET /api/projects/{project}/agent/catalog", s.handleAgentCatalog)
	scoped("GET /api/projects/{project}/agent/capabilities", s.handleAgentCapabilities)

	// Execution dispatch — legacy
	legacy("POST /api/exec/run/{id}", s.handleExecDispatch)
	legacy("POST /api/agent/run", s.handleAgentDispatch)
	legacy("GET /api/agent/workers", s.handleAgentWorkers)
	trusted("POST /api/agent/workers/execute-loop", s.handleStartExecuteLoopWorker)
	trusted("POST /api/agent/workers/prune", s.handlePruneAgentWorkers)
	legacy("GET /api/agent/workers/{id}", s.handleAgentWorkerShow)
	trusted("POST /api/agent/workers/{id}/stop", s.handleStopAgentWorker)
	legacy("GET /api/agent/workers/{id}/log", s.handleAgentWorkerLog)
	legacy("GET /api/agent/workers/{id}/prompt", s.handleAgentWorkerPrompt)
	legacy("GET /api/agent/coordinators", s.handleAgentCoordinators)

	// Agent — project-scoped (FEAT-002: canonical)
	scoped("POST /api/projects/{project}/agent/run", s.handleAgentDispatch)
	scoped("GET /api/projects/{project}/agent/workers", s.handleAgentWorkers)
	scoped("GET /api/projects/{project}/agent/workers/{id}", s.handleAgentWorkerShow)
	scoped("GET /api/projects/{project}/agent/workers/{id}/log", s.handleAgentWorkerLog)
	scoped("GET /api/projects/{project}/agent/workers/{id}/prompt", s.handleAgentWorkerPrompt)
	scoped("GET /api/projects/{project}/agent/coordinators", s.handleAgentCoordinators)

	// Worker ingestion (ADR-022 rev 5 §Worker-server interface). Workers
	// POST best-effort to register, mirror events, and backfill events
	// buffered while NotConnected. Project membership is carried in the
	// register payload — these endpoints are NOT project-scoped.
	trusted("POST /api/workers/register", s.handleWorkerRegister)
	trusted("POST /api/workers/{id}/event", s.handleWorkerEvent)
	trusted("POST /api/workers/{id}/backfill", s.handleWorkerBackfill)
	trusted("GET /api/workers", s.handleWorkerIngestList)

	// Project-scoped worker endpoints (FEAT-002 §22-24)
	trusted("GET /api/projects/{project}/workers", s.handleProjectWorkerList)
	trusted("GET /api/projects/{project}/workers/{id}/progress", s.handleProjectWorkerProgress)
	trusted("GET /api/projects/{project}/workers/{id}", s.handleProjectWorkerShow)

	// Executions — legacy
	legacy("GET /api/exec/definitions/{id}", s.handleExecDefinitionShow)
	legacy("GET /api/exec/definitions", s.handleExecDefinitions)
	legacy("GET /api/exec/runs/{id}/log", s.handleExecRunLog)
	legacy("GET /api/exec/runs/{id}", s.handleExecRunShow)
	legacy("GET /api/exec/runs", s.handleExecRuns)

	// Executions — project-scoped (FEAT-002: canonical)
	scoped("POST /api/projects/{project}/exec/run/{id}", s.handleExecDispatch)
	scoped("GET /api/projects/{project}/exec/definitions/{id}", s.handleExecDefinitionShow)
	scoped("GET /api/projects/{project}/exec/definitions", s.handleExecDefinitions)
	scoped("GET /api/projects/{project}/exec/runs/{id}/log", s.handleExecRunLog)
	scoped("GET /api/projects/{project}/exec/runs/{id}", s.handleExecRunShow)
	scoped("GET /api/projects/{project}/exec/runs", s.handleExecRuns)

	// Run evidence-bundle file download (Story 16). Streams a single file from
	// the run's bundle directory after canonicalisation + confinement checks.
	trusted("GET /api/runs/{id}/bundle", s.handleRunBundleDownload)
	scoped("GET /api/projects/{project}/runs/{id}/bundle", s.handleRunBundleDownload)

	// Agent sessions — legacy
	legacy("GET /api/agent/sessions/{id}", s.handleAgentSessionDetail)
	legacy("GET /api/agent/sessions", s.handleAgentSessions)

	// Agent sessions — project-scoped (FEAT-002: canonical)
	scoped("GET /api/projects/{project}/agent/sessions/{id}", s.handleAgentSessionDetail)
	scoped("GET /api/projects/{project}/agent/sessions", s.handleAgentSessions)

	// Process metrics — legacy
	legacy("GET /api/metrics/summary", s.handleMetricsSummary)
	legacy("GET /api/metrics/cost", s.handleMetricsCost)
	legacy("GET /api/metrics/cycle-time", s.handleMetricsCycleTime)
	legacy("GET /api/metrics/rework", s.handleMetricsRework)

	// Process metrics — project-scoped (FEAT-002: canonical)
	scoped("GET /api/projects/{project}/metrics/summary", s.handleMetricsSummary)
	scoped("GET /api/projects/{project}/metrics/cost", s.handleMetricsCost)
	scoped("GET /api/projects/{project}/metrics/cycle-time", s.handleMetricsCycleTime)
	scoped("GET /api/projects/{project}/metrics/rework", s.handleMetricsRework)

	// Per-metric-id history and trend — legacy
	legacy("GET /api/metrics/{id}/history", s.handleMetricHistory)
	legacy("GET /api/metrics/{id}/trend", s.handleMetricTrend)

	// Per-metric-id history and trend — project-scoped (FEAT-016)
	scoped("GET /api/projects/{project}/metrics/{id}/history", s.handleMetricHistory)
	scoped("GET /api/projects/{project}/metrics/{id}/trend", s.handleMetricTrend)

	// Providers (FEAT-002 §26-27, host+user global — not project-scoped)
	trusted("GET /api/providers", s.handleListProviders)
	trusted("GET /api/providers/{harness}", s.handleShowProvider)

	// MCP server registry + plugin manifest (FEAT-009/015 read coverage)
	trusted("GET /api/mcp-servers", s.handleListMCPServers)
	trusted("GET /api/plugins", s.handleListPlugins)

	// MCP
	trusted("POST /mcp", s.handleMCP)

	// GraphQL (gqlgen) — POST for queries/mutations, GET for WebSocket subscriptions
	trusted("POST /graphql", s.handleGraphQLQuery)
	trusted("GET /graphql", s.handleGraphQLQuery)
	trusted("GET /graphiql", s.handleGraphiQL)
	// LAYER 1 scoped GraphQL endpoint (ddx-4c51d33e). Closes the HIGH-severity
	// DocumentByPath cross-project leak by serving each request against the
	// resolved {project}'s WorkingDir instead of the server's startup project.
	scoped("POST /api/projects/{project}/graphql", s.handleGraphQLQueryScoped)
	scoped("GET /api/projects/{project}/graphql", s.handleGraphQLQueryScoped)

	// Story 8: Sessions and Executions tabs were merged into the layer-aware
	// Runs row expansion. Direct/bookmarked URLs return a 302 with a Sunset
	// header so existing tooling/links keep working through the deprecation
	// window. Client-side navigations are handled by the corresponding
	// SvelteKit +page.ts redirects.
	trusted("GET /nodes/{nodeId}/projects/{projectId}/sessions",
		runsRedirectHandler(func(r *http.Request) string {
			q := r.URL.Query()
			q.Set("layer", "run")
			return fmt.Sprintf("/nodes/%s/projects/%s/runs?%s",
				r.PathValue("nodeId"), r.PathValue("projectId"), q.Encode())
		}))
	trusted("GET /nodes/{nodeId}/projects/{projectId}/executions",
		runsRedirectHandler(func(r *http.Request) string {
			q := url.Values{}
			if h := r.URL.Query().Get("harness"); h != "" {
				q.Set("harness", h)
			}
			q.Set("layer", "try")
			return fmt.Sprintf("/nodes/%s/projects/%s/runs?%s",
				r.PathValue("nodeId"), r.PathValue("projectId"), q.Encode())
		}))
	trusted("GET /nodes/{nodeId}/projects/{projectId}/executions/{execId}",
		runsRedirectHandler(func(r *http.Request) string {
			execID := r.PathValue("execId")
			runID := execID
			if !strings.HasPrefix(runID, "exec-") {
				runID = "exec-" + runID
			}
			return fmt.Sprintf("/nodes/%s/projects/%s/runs/%s",
				r.PathValue("nodeId"), r.PathValue("projectId"), runID)
		}))

	// SvelteKit SPA — serve embedded frontend/build; fall back to index.html for deep links.
	sub, err := fs.Sub(frontendFiles, "frontend/build")
	if err != nil {
		panic("embed: frontend/build not found: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	trusted("/", func(w http.ResponseWriter, r *http.Request) {
		// Root: let the file server resolve index.html directly.
		if r.URL.Path == "" || r.URL.Path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Attempt to serve the exact file. If not found, serve index.html so
		// the SvelteKit client-side router can handle the URL.
		_, statErr := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/"))
		if statErr != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// --- Health Endpoints ---

func (s *Server) handleGetNode(w http.ResponseWriter, _ *http.Request) {
	s.state.mu.RLock()
	node := s.state.Node
	s.state.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":            node.Name,
		"id":              node.ID,
		"started_at":      node.StartedAt,
		"last_seen":       node.LastSeen,
		"federation_role": s.federationRole(),
	})
}

// federationRole returns the node's federation role string for /api/node.
//   - "standalone": neither hub nor spoke mode is enabled
//   - "hub":        hub-mode only
//   - "spoke":      spoke-mode only
//   - "hub_spoke":  both
func (s *Server) federationRole() string {
	switch {
	case s.HubMode && s.SpokeMode:
		return "hub_spoke"
	case s.HubMode:
		return "hub"
	case s.SpokeMode:
		return "spoke"
	default:
		return "standalone"
	}
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	includeUnreachable := r.URL.Query().Get("include_unreachable") == "true"
	writeJSON(w, http.StatusOK, s.state.GetProjects(includeUnreachable))
}

func (s *Server) handleRegisterProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}
	entry := s.state.RegisterProject(body.Path)
	_ = s.state.save()
	writeJSON(w, http.StatusOK, entry)
}

// commitEntry is the JSON shape returned by the commits endpoint.
type commitEntry struct {
	SHA      string   `json:"sha"`
	ShortSHA string   `json:"short_sha"`
	Author   string   `json:"author"`
	Date     string   `json:"date"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	BeadRefs []string `json:"bead_refs"`
}

// beadRefRegex matches bead IDs of the form ddx-<8 hex chars>.
var beadRefRegex = regexp.MustCompile(`ddx-[a-f0-9]{8}`)

func (s *Server) handleProjectCommits(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("project")
	entry, ok := s.state.GetProjectByID(key)
	if !ok {
		entry, ok = s.state.GetProjectByPath(key)
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	since := r.URL.Query().Get("since")
	author := r.URL.Query().Get("author")

	// Use ASCII control chars for delimiters to avoid collisions with commit
	// message content. NUL cannot be used because Go's exec package rejects
	// it in argv. \x1f = unit separator, \x1e = record separator.
	const sep = "\x1f"
	const recSep = "\x1e"
	format := "--pretty=format:%H" + sep + "%h" + sep + "%an" + sep + "%aI" + sep + "%s" + sep + "%b" + recSep

	args := []string{"log", format, "-n", strconv.Itoa(limit)}
	if since != "" {
		args = append(args, "--since="+since)
	}
	if author != "" {
		args = append(args, "--author="+author)
	}

	cmd := internalgit.Command(r.Context(), entry.Path, args...)
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "git log failed: " + err.Error()})
		return
	}

	commits := []commitEntry{}
	records := strings.Split(string(out), recSep)
	for _, rec := range records {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		fields := strings.SplitN(rec, sep, 6)
		if len(fields) < 6 {
			continue
		}
		body := fields[5]
		refs := beadRefRegex.FindAllString(fields[4]+"\n"+body, -1)
		if refs == nil {
			refs = []string{}
		}
		commits = append(commits, commitEntry{
			SHA:      fields[0],
			ShortSHA: fields[1],
			Author:   fields[2],
			Date:     fields[3],
			Subject:  fields[4],
			Body:     body,
			BeadRefs: refs,
		})
	}

	writeJSON(w, http.StatusOK, commits)
}

func (s *Server) handleCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := ""
	if s.csrfTokens != nil {
		token = s.csrfTokens.Token()
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"started_at": s.startTime.Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	ready := true

	// Check library path
	if s.libraryPath() != "" {
		checks["library"] = "ok"
	} else {
		checks["library"] = "not_configured"
	}

	// Check bead store
	store := s.beadStore()
	if _, err := store.Status(); err != nil {
		checks["beads"] = "error: " + err.Error()
		ready = false
	} else {
		checks["beads"] = "ok"
	}

	// Check doc graph
	if _, err := s.buildDocGraph(); err != nil {
		checks["docgraph"] = "error: " + err.Error()
	} else {
		checks["docgraph"] = "ok"
	}

	status := http.StatusOK
	statusStr := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		statusStr = "not_ready"
	}
	writeJSON(w, status, map[string]any{
		"status": statusStr,
		"checks": checks,
	})
}

// --- Document Endpoints ---

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	libPath := s.libraryPathForRequest(r)
	if libPath == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	type docEntry struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}

	var docs []docEntry
	categories := []string{"prompts", "templates", "personas", "patterns", "configs", "scripts", "mcp-servers"}
	typeFilter := r.URL.Query().Get("type")

	for _, cat := range categories {
		if typeFilter != "" && cat != typeFilter {
			continue
		}
		catDir := filepath.Join(libPath, cat)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			docs = append(docs, docEntry{
				Name: e.Name(),
				Type: cat,
				Path: filepath.Join(cat, e.Name()),
			})
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Type != docs[j].Type {
			return docs[i].Type < docs[j].Type
		}
		return docs[i].Name < docs[j].Name
	})
	writeJSON(w, http.StatusOK, docs)
}

func (s *Server) handleReadDocument(w http.ResponseWriter, r *http.Request) {
	docPath := r.PathValue("path")
	if docPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}

	libPath := s.libraryPathForRequest(r)
	if libPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "library not configured"})
		return
	}

	// Prevent path traversal
	cleaned := filepath.Clean(docPath)
	if strings.Contains(cleaned, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	fullPath := filepath.Join(libPath, cleaned)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	clamped, truncated, originalBytes := evidence.ClampOutput(string(data), serverInlineCapBytes)
	writeJSON(w, http.StatusOK, map[string]any{
		"path":           cleaned,
		"content":        clamped,
		"truncated":      truncated,
		"original_bytes": originalBytes,
	})
}

func (s *Server) handleWriteDocument(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	docPath := r.PathValue("path")
	if docPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}

	libPath := s.libraryPathForRequest(r)
	if libPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "library not configured"})
		return
	}

	cleaned := filepath.Clean(docPath)
	if strings.Contains(cleaned, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	fullPath := filepath.Join(libPath, cleaned)
	if err := os.WriteFile(fullPath, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": cleaned, "status": "saved"})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q parameter required"})
		return
	}

	libPath := s.libraryPathForRequest(r)
	if libPath == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	type searchResult struct {
		Path    string `json:"path"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Snippet string `json:"snippet,omitempty"`
	}

	var results []searchResult
	queryLower := strings.ToLower(query)
	categories := []string{"prompts", "templates", "personas", "patterns", "configs", "scripts", "mcp-servers"}

	for _, cat := range categories {
		catDir := filepath.Join(libPath, cat)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			nameLower := strings.ToLower(e.Name())
			relPath := filepath.Join(cat, e.Name())
			fullPath := filepath.Join(libPath, relPath)

			// Check filename match
			nameMatch := strings.Contains(nameLower, queryLower)

			// Check content match
			var snippet string
			if data, err := os.ReadFile(fullPath); err == nil {
				contentLower := strings.ToLower(string(data))
				if idx := strings.Index(contentLower, queryLower); idx >= 0 {
					start := idx - 40
					if start < 0 {
						start = 0
					}
					end := idx + len(query) + 40
					if end > len(data) {
						end = len(data)
					}
					snippet = strings.TrimSpace(string(data[start:end]))
				}
			}

			if nameMatch || snippet != "" {
				results = append(results, searchResult{
					Path:    relPath,
					Type:    cat,
					Name:    e.Name(),
					Snippet: snippet,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleListPersonas(w http.ResponseWriter, r *http.Request) {
	loader := persona.NewPersonaLoader(s.WorkingDir)
	personas, err := loader.ListPersonas()
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	result := make([]map[string]any, 0, len(personas))
	for _, p := range personas {
		result = append(result, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"roles":       p.Roles,
			"tags":        p.Tags,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleResolvePersona(w http.ResponseWriter, r *http.Request) {
	role := r.PathValue("role")
	if role == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role required"})
		return
	}

	bm := persona.NewBindingManagerWithPath(filepath.Join(s.WorkingDir, ".ddx.yml"))
	personaName, err := bm.GetBinding(role)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("no persona bound to role: %s", role)})
		return
	}

	loader := persona.NewPersonaLoader(s.WorkingDir)
	p, err := loader.LoadPersona(personaName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("persona not found: %s", personaName)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"role":        role,
		"persona":     p.Name,
		"description": p.Description,
		"roles":       p.Roles,
		"tags":        p.Tags,
		"content":     p.Content,
	})
}

// --- Bead Endpoints ---

// beadWithProject wraps a Bead with its source project ID for cross-project responses.
type beadWithProject struct {
	bead.Bead
	ProjectID string `json:"project_id"`
}

func (s *Server) handleListBeads(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	label := r.URL.Query().Get("label")
	projectFilter := r.URL.Query().Get("project_id")

	// Scoped route injects a project into context; pin the filter to it so the
	// handler only emits beads from that project.
	if p, ok := projectFromContext(r.Context()); ok {
		projectFilter = p.ID
	}

	projects := s.state.GetProjects()
	var result []beadWithProject

	for _, proj := range projects {
		if projectFilter != "" && proj.ID != projectFilter {
			continue
		}
		store := bead.NewStore(filepath.Join(proj.Path, ".ddx"))
		beads, err := store.ReadAll()
		if err != nil {
			continue
		}
		for _, b := range beads {
			if status != "" && b.Status != status {
				continue
			}
			if label != "" && !containsString(b.Labels, label) {
				continue
			}
			result = append(result, beadWithProject{Bead: b, ProjectID: proj.ID})
		}
	}

	if result == nil {
		result = []beadWithProject{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleShowBead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	store := s.beadStoreForRequest(r)
	b, err := store.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bead not found"})
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleBeadsReady(w http.ResponseWriter, r *http.Request) {
	store := s.beadStoreForRequest(r)
	ready, err := store.Ready()
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	if ready == nil {
		ready = []bead.Bead{}
	}
	writeJSON(w, http.StatusOK, ready)
}

func (s *Server) handleBeadsBlocked(w http.ResponseWriter, r *http.Request) {
	store := s.beadStoreForRequest(r)
	blocked, err := store.Blocked()
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	if blocked == nil {
		blocked = []bead.Bead{}
	}
	writeJSON(w, http.StatusOK, blocked)
}

func (s *Server) handleBeadsStatus(w http.ResponseWriter, r *http.Request) {
	store := s.beadStoreForRequest(r)
	counts, err := store.Status()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, counts)
}

func (s *Server) handleBeadDepTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	store := s.beadStoreForRequest(r)
	tree, err := store.DepTree(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "tree": tree})
}

func (s *Server) handleBeadEvidence(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	store := s.beadStoreForRequest(r)
	events, err := store.Events(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bead not found"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleBeadCooldown(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	store := s.beadStoreForRequest(r)
	b, err := store.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bead not found"})
		return
	}
	retry, _ := b.Extra["execute-loop-retry-after"].(string)
	lastStatus, _ := b.Extra["execute-loop-last-status"].(string)
	lastDetail, _ := b.Extra["execute-loop-last-detail"].(string)
	writeJSON(w, http.StatusOK, struct {
		BeadID     string `json:"bead_id"`
		RetryAfter string `json:"retry_after,omitempty"`
		LastStatus string `json:"last_status,omitempty"`
		LastDetail string `json:"last_detail,omitempty"`
	}{BeadID: b.ID, RetryAfter: retry, LastStatus: lastStatus, LastDetail: lastDetail})
}

func (s *Server) handleBeadRouting(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	store := s.beadStoreForRequest(r)
	events, err := store.EventsByKind(id, "routing")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bead not found"})
		return
	}
	parsed := make([]beadRoutingEvent, 0, len(events))
	for _, e := range events {
		re := beadRoutingEvent{CreatedAt: e.CreatedAt, Summary: e.Summary}
		if e.Body != "" {
			_ = json.Unmarshal([]byte(e.Body), &re)
		}
		parsed = append(parsed, re)
	}
	writeJSON(w, http.StatusOK, parsed)
}

// --- Bead Mutation Endpoints ---

func (s *Server) handleCreateBead(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	var req struct {
		Title       string   `json:"title"`
		Type        string   `json:"type"`
		Priority    *int     `json:"priority"`
		Labels      []string `json:"labels"`
		Description string   `json:"description"`
		Acceptance  string   `json:"acceptance"`
		Parent      string   `json:"parent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}

	store := s.beadStoreForRequest(r)
	b := &bead.Bead{
		Title:       req.Title,
		IssueType:   req.Type,
		Labels:      req.Labels,
		Description: req.Description,
		Acceptance:  req.Acceptance,
		Parent:      req.Parent,
	}
	if req.Priority != nil {
		b.Priority = *req.Priority
	}
	if err := store.Create(b); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleUpdateBead(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	var req struct {
		Status      *string  `json:"status"`
		Labels      []string `json:"labels"`
		Description *string  `json:"description"`
		Acceptance  *string  `json:"acceptance"`
		Priority    *int     `json:"priority"`
		Notes       *string  `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	store := s.beadStoreForRequest(r)
	err := store.Update(id, func(b *bead.Bead) {
		if req.Status != nil {
			b.Status = *req.Status
		}
		if req.Labels != nil {
			b.Labels = req.Labels
		}
		if req.Description != nil {
			b.Description = *req.Description
		}
		if req.Acceptance != nil {
			b.Acceptance = *req.Acceptance
		}
		if req.Priority != nil {
			b.Priority = *req.Priority
		}
		if req.Notes != nil {
			b.Notes = *req.Notes
		}
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	updated, err := store.Get(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleClaimBead(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	var req struct {
		Assignee string `json:"assignee"`
		Session  string `json:"session"`
		Worktree string `json:"worktree"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	store := s.beadStoreForRequest(r)
	if err := store.ClaimWithOptions(id, req.Assignee, req.Session, req.Worktree); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "claimed"})
}

func (s *Server) handleUnclaimBead(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	store := s.beadStoreForRequest(r)
	if err := store.Unclaim(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "unclaimed"})
}

func (s *Server) handleReopenBead(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	store := s.beadStoreForRequest(r)
	err := store.Update(id, func(b *bead.Bead) {
		b.Status = bead.StatusOpen
		b.Owner = ""
		if req.Reason != "" && b.Notes != "" {
			b.Notes = b.Notes + "\n\nReopened: " + req.Reason
		} else if req.Reason != "" {
			b.Notes = "Reopened: " + req.Reason
		}
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "reopened"})
}

// handleCancelBead writes Extra[cancel-requested]=true on the bead. ADR-022
// §Cancel SLA: workers running an attempt for this bead poll the marker every
// CancelPollInterval (default 10s) and abort at the next safe point with
// reason "operator_cancel". Idempotent: a second cancel after the worker has
// honored the first is a silent no-op (the worker writes
// cancel-honored:true alongside the marker).
func (s *Server) handleCancelBead(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	store := s.beadStoreForRequest(r)
	if _, err := store.RequestCancel(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "cancel-requested"})
}

func (s *Server) handleBeadDeps(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	var req struct {
		Action string `json:"action"` // "add" or "remove"
		DepID  string `json:"dep_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.DepID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dep_id is required"})
		return
	}

	store := s.beadStoreForRequest(r)
	var err error
	switch req.Action {
	case "add":
		err = store.DepAdd(id, req.DepID)
	case "remove":
		err = store.DepRemove(id, req.DepID)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be 'add' or 'remove'"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "action": req.Action, "dep_id": req.DepID})
}

// --- Doc Graph Endpoints ---

func (s *Server) handleDocGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type graphNode struct {
		ID         string   `json:"id"`
		Path       string   `json:"path"`
		DependsOn  []string `json:"depends_on,omitempty"`
		Dependents []string `json:"dependents,omitempty"`
	}
	nodes := make([]graphNode, 0, len(graph.Documents))
	for _, doc := range graph.Documents {
		nodes = append(nodes, graphNode{
			ID:         doc.ID,
			Path:       doc.Path,
			DependsOn:  doc.DependsOn,
			Dependents: doc.Dependents,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleDocStale(w http.ResponseWriter, r *http.Request) {
	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	stale := graph.StaleDocs()
	if stale == nil {
		stale = []docgraph.StaleReason{}
	}
	writeJSON(w, http.StatusOK, stale)
}

func (s *Server) handleDocChanged(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	wd := s.workingDirForRequest(r)
	result := s.mcpDocChanged(wd, since)
	if result.IsError {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": result.Content[0].Text})
		return
	}
	// mcpDocChanged returns JSON-encoded array as text; decode and re-encode for HTTP.
	var entries any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &entries); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleDocShow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, ok := graph.Show(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	staleReason, isStale := graph.StaleReasonForID(id)
	resp := map[string]any{
		"id":         doc.ID,
		"path":       doc.Path,
		"title":      doc.Title,
		"depends_on": doc.DependsOn,
		"dependents": doc.Dependents,
		"is_stale":   isStale,
	}
	if isStale {
		resp["stale_reasons"] = staleReason.Reasons
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDocDeps(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	deps, err := graph.Dependencies(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, deps)
}

func (s *Server) handleDocDependents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	dependents, err := graph.DependentIDs(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, dependents)
}

func (s *Server) handleDocWrite(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	wd := s.workingDirForRequest(r)
	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, ok := graph.Show(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	// doc.Path is relative to the docgraph root; resolve against workingDir
	// before touching the file system.
	fullPath := doc.Path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(wd, fullPath)
	}
	if err := os.WriteFile(fullPath, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	committed := false
	var acCfg internalgit.AutoCommitConfig
	if cfg, cfgErr := config.LoadWithWorkingDir(wd); cfgErr == nil && cfg.Git != nil {
		acCfg.AutoCommit = cfg.Git.AutoCommit
		acCfg.CommitPrefix = cfg.Git.CommitPrefix
	}
	if acCfg.AutoCommit == "always" {
		if _, acErr := internalgit.AutoCommit(fullPath, id, "write document", acCfg); acErr == nil {
			committed = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"path":      doc.Path,
		"committed": committed,
	})
}

func (s *Server) handleDocHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, ok := graph.Show(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	gitArgs := []string{"log", "--follow", "--format=%H\t%ai\t%an\t%s", "--", doc.Path}
	gitCmd := internalgit.Command(r.Context(), s.workingDirForRequest(r), gitArgs...)
	out, gitErr := gitCmd.Output()
	if gitErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "git log failed"})
		return
	}

	type commitEntry struct {
		Hash    string `json:"hash"`
		Date    string `json:"date"`
		Author  string `json:"author"`
		Message string `json:"message"`
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	entries := make([]commitEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		hash := parts[0]
		if len(hash) > 7 {
			hash = hash[:7]
		}
		date := parts[1]
		if len(date) > 10 {
			date = date[:10]
		}
		entries = append(entries, commitEntry{
			Hash:    hash,
			Date:    date,
			Author:  parts[2],
			Message: parts[3],
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleDocDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	graph, err := s.buildDocGraphForRequest(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, ok := graph.Show(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	ref := r.URL.Query().Get("ref")
	var gitArgs []string
	if ref != "" {
		gitArgs = []string{"diff", ref, "--", doc.Path}
	} else {
		gitArgs = []string{"diff", "--", doc.Path}
	}

	diffCmd := internalgit.Command(r.Context(), s.workingDirForRequest(r), gitArgs...)
	out, gitErr := diffCmd.Output()
	if gitErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "git diff failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"diff": string(out)})
}

// --- Execution Endpoints ---

func (s *Server) execStore() *ddxexec.Store {
	return ddxexec.NewStore(s.WorkingDir)
}

func (s *Server) handleExecDefinitions(w http.ResponseWriter, r *http.Request) {
	store := s.execStoreForRequest(r)
	artifactID := r.URL.Query().Get("artifact")
	defs, err := store.ListDefinitions(artifactID)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, defs)
}

func (s *Server) handleExecDefinitionShow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	store := s.execStoreForRequest(r)
	def, err := store.ShowDefinition(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (s *Server) handleExecRuns(w http.ResponseWriter, r *http.Request) {
	store := s.execStoreForRequest(r)
	artifactID := r.URL.Query().Get("artifact")
	definitionID := r.URL.Query().Get("definition")
	runs, err := store.History(artifactID, definitionID)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleExecRunShow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	store := s.execStoreForRequest(r)
	result, err := store.Result(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExecRunLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	store := s.execStoreForRequest(r)
	stdout, stderr, err := store.Log(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stdout": stdout, "stderr": stderr})
}

// --- Dispatch Endpoints (localhost-only) ---

func isLocalhost(r *http.Request) bool {
	return isLocalhostAddr(r.RemoteAddr)
}

func (s *Server) handleExecDispatch(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dispatch endpoints are localhost-only"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "definition id is required"})
		return
	}

	store := s.execStoreForRequest(r)
	record, err := store.Run(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleAgentDispatch(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dispatch endpoints are localhost-only"})
		return
	}

	var req struct {
		Harness string `json:"harness"`
		Prompt  string `json:"prompt"`
		Model   string `json:"model"`
		Effort  string `json:"effort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Harness == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "harness is required"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	if observed := len([]byte(req.Prompt)); observed > serverPromptCapBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error":          fmt.Sprintf("inline prompt body exceeds cap: observed %d bytes, cap %d bytes", observed, serverPromptCapBytes),
			"observed_bytes": observed,
			"cap_bytes":      serverPromptCapBytes,
			"config_hint":    ".ddx/config.yaml:evidence_caps.max_prompt_bytes",
		})
		return
	}

	workDir := s.workingDirForRequest(r)
	rcfg, _ := config.LoadAndResolve(workDir, config.CLIOverrides{
		Harness: req.Harness,
		Model:   req.Model,
		Effort:  req.Effort,
	})
	runtime := agent.AgentRunRuntime{
		Prompt:  req.Prompt,
		WorkDir: workDir,
	}
	result, err := agent.RunWithConfigViaService(r.Context(), workDir, rcfg, runtime)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAgentWorkers(w http.ResponseWriter, r *http.Request) {
	// Scoped/singleton: return workers for just the resolved project.
	if _, ok := projectFromContext(r.Context()); ok {
		m := s.workerManagerForRequest(r)
		recs, err := m.List()
		if err != nil {
			writeJSON(w, http.StatusOK, []WorkerRecord{})
			return
		}
		sort.Slice(recs, func(i, j int) bool {
			return recs[i].StartedAt.After(recs[j].StartedAt)
		})
		writeJSON(w, http.StatusOK, recs)
		return
	}

	projects := s.state.GetProjects()

	var all []WorkerRecord
	seen := map[string]bool{}

	for _, proj := range projects {
		var m *WorkerManager
		if proj.Path == s.WorkingDir {
			m = s.workers
		} else {
			m = NewWorkerManager(proj.Path)
		}
		recs, err := m.List()
		if err != nil {
			continue
		}
		for _, rec := range recs {
			if !seen[rec.ID] {
				all = append(all, rec)
				seen[rec.ID] = true
			}
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].StartedAt.After(all[j].StartedAt)
	})

	writeJSON(w, http.StatusOK, all)
}

func (s *Server) handleAgentCoordinators(w http.ResponseWriter, r *http.Request) {
	if p, ok := projectFromContext(r.Context()); ok {
		if m := s.workers.LandCoordinators.MetricsFor(p.Path); m != nil {
			writeJSON(w, http.StatusOK, []CoordinatorMetricsEntry{{
				ProjectRoot: p.Path,
				Metrics:     *m,
			}})
			return
		}
		writeJSON(w, http.StatusOK, []CoordinatorMetricsEntry{})
		return
	}
	writeJSON(w, http.StatusOK, s.workers.LandCoordinators.AllMetrics())
}

func (s *Server) handleStartExecuteLoopWorker(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dispatch endpoints are localhost-only"})
		return
	}
	var req struct {
		ProjectRoot   string `json:"project_root"`
		Harness       string `json:"harness"`
		Model         string `json:"model"`
		Profile       string `json:"profile"`
		Provider      string `json:"provider"`
		ModelRef      string `json:"model_ref"`
		Effort        string `json:"effort"`
		LabelFilter   string `json:"label_filter"`
		Once          bool   `json:"once"`
		PollInterval  string `json:"poll_interval"`
		NoReview      bool   `json:"no_review"`
		ReviewHarness string `json:"review_harness"`
		ReviewModel   string `json:"review_model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Resolve project_root: if provided, validate it against registered projects.
	// An empty project_root means "use the server's primary project" (default).
	projectRoot := req.ProjectRoot
	if projectRoot != "" {
		resolved := s.resolveRequestedProject(projectRoot)
		if resolved == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": fmt.Sprintf("no registered project matches %q; ensure the server was started from or has registered that project root", projectRoot),
			})
			return
		}
		projectRoot = resolved
	}

	// Default poll-interval = 30s for server-managed workers (ddx-dc157075).
	// Operators wanting drain-and-exit semantics must pass --once or an
	// explicit poll_interval=0 in the dispatch payload.
	pollInterval := 30 * time.Second
	if strings.TrimSpace(req.PollInterval) != "" {
		d, err := time.ParseDuration(req.PollInterval)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid poll_interval"})
			return
		}
		pollInterval = d
	}
	record, err := s.workers.StartExecuteLoop(ExecuteLoopWorkerSpec{
		ProjectRoot:   projectRoot,
		Harness:       req.Harness,
		Model:         req.Model,
		Profile:       req.Profile,
		Provider:      req.Provider,
		ModelRef:      req.ModelRef,
		Effort:        req.Effort,
		LabelFilter:   req.LabelFilter,
		Once:          req.Once,
		PollInterval:  pollInterval,
		NoReview:      req.NoReview,
		ReviewHarness: req.ReviewHarness,
		ReviewModel:   req.ReviewModel,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

// resolveRequestedProject matches a requested project_root string against
// registered projects. It tries exact path match first, then name (basename)
// match. Returns the canonical path on success, or "" if no match or ambiguous.
func (s *Server) resolveRequestedProject(requested string) string {
	// Exact path match via the state's lookup method.
	if entry, ok := s.state.GetProjectByPath(requested); ok {
		return entry.Path
	}
	// Name (basename) match — only unambiguous if exactly one project has that name.
	projects := s.state.GetProjects()
	var matches []string
	for _, p := range projects {
		if p.Name == requested {
			matches = append(matches, p.Path)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func (s *Server) handleAgentWorkerShow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	m := s.workerManagerForRequest(r)
	record, err := m.Show(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	// Attach coordinator land summary for this worker's project.
	if record.ProjectRoot != "" {
		metrics := s.workers.LandCoordinators.MetricsFor(record.ProjectRoot)
		if metrics != nil {
			record.LandSummary = metrics
		}
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleStopAgentWorker(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dispatch endpoints are localhost-only"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	if err := s.workers.Stop(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "stopping"})
}

func (s *Server) handlePruneAgentWorkers(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dispatch endpoints are localhost-only"})
		return
	}
	var maxAge time.Duration
	if v := r.URL.Query().Get("max_age"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_age: " + err.Error()})
			return
		}
		maxAge = d
	}
	m := s.workerManagerForRequest(r)
	results, err := m.Prune(maxAge)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if results == nil {
		results = []WorkerPruneResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleAgentWorkerLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	m := s.workerManagerForRequest(r)
	stdout, stderr, err := m.Logs(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stdout": stdout, "stderr": stderr})
}

func (s *Server) handleAgentWorkerPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	m := s.workerManagerForRequest(r)
	record, err := m.Show(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	// Try current attempt first, fall back to last attempt
	var attemptID string
	if record.CurrentAttempt != nil {
		attemptID = record.CurrentAttempt.AttemptID
	} else if record.LastAttempt != nil {
		attemptID = record.LastAttempt.AttemptID
	}
	if attemptID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no attempt available"})
		return
	}
	promptPath := filepath.Join(s.workingDirForRequest(r), ".ddx", "executions", attemptID, "prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "prompt not found"})
		return
	}
	clamped, truncated, originalBytes := evidence.ClampOutput(string(data), serverInlineCapBytes)
	writeJSON(w, http.StatusOK, map[string]any{
		"prompt":         clamped,
		"truncated":      truncated,
		"original_bytes": originalBytes,
	})
}

// --- Project-Scoped Worker Endpoints (FEAT-002 §22-24) ---

// resolveWorkerManager returns the WorkerManager for the given project key
// (by ID or path). If the resolved project matches the server's own working
// directory it returns the live s.workers; otherwise it returns a read-only
// manager backed only by disk state.
func (s *Server) resolveWorkerManager(projectKey string) (*WorkerManager, bool) {
	entry, ok := s.state.GetProjectByID(projectKey)
	if !ok {
		entry, ok = s.state.GetProjectByPath(projectKey)
	}
	if !ok {
		return nil, false
	}
	if entry.Path == s.WorkingDir {
		return s.workers, true
	}
	// Return a read-only manager for the registered project (no live workers)
	m := NewWorkerManager(entry.Path)
	return m, true
}

func (s *Server) handleProjectWorkerList(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("project")
	m, ok := s.resolveWorkerManager(key)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	workers, err := m.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, workers)
}

func (s *Server) handleProjectWorkerShow(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("project")
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	m, ok := s.resolveWorkerManager(key)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	record, err := m.Show(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// handleProjectWorkerProgress streams FEAT-006 progress events as Server-Sent
// Events for the given worker. When no run is active it emits keepalive
// comment lines at a fixed interval. The stream closes when:
//   - the attempt reaches a terminal phase (done, preserved, failed), or
//   - the client disconnects.
func (s *Server) handleProjectWorkerProgress(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("project")
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	m, ok := s.resolveWorkerManager(key)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	// Verify the worker exists before subscribing.
	if _, err := m.Show(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := m.SubscribeProgress(id)
	defer unsub()

	const keepaliveInterval = 30 * time.Second
	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, open := <-ch:
			if !open {
				// Worker finished and channel was closed — send keepalive and return
				_, _ = fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if terminalPhases[evt.Phase] {
				return
			}
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// --- Agent Session Endpoints ---

type agentSessionSummary struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Harness         string            `json:"harness"`
	Surface         string            `json:"surface,omitempty"`
	CanonicalTarget string            `json:"canonical_target,omitempty"`
	Model           string            `json:"model,omitempty"`
	PromptLen       int               `json:"prompt_len"`
	PromptSource    string            `json:"prompt_source,omitempty"`
	Correlation     map[string]string `json:"correlation,omitempty"`
	NativeSessionID string            `json:"native_session_id,omitempty"`
	NativeLogRef    string            `json:"native_log_ref,omitempty"`
	TraceID         string            `json:"trace_id,omitempty"`
	SpanID          string            `json:"span_id,omitempty"`
	Stderr          string            `json:"stderr,omitempty"`
	Tokens          int               `json:"tokens,omitempty"`
	InputTokens     int               `json:"input_tokens,omitempty"`
	OutputTokens    int               `json:"output_tokens,omitempty"`
	CostUSD         float64           `json:"cost_usd,omitempty"`
	DurationMS      int               `json:"duration_ms"`
	ExitCode        int               `json:"exit_code"`
	Error           string            `json:"error,omitempty"`
	TotalTokens     int               `json:"total_tokens,omitempty"`
	BaseRev         string            `json:"base_rev,omitempty"`
	ResultRev       string            `json:"result_rev,omitempty"`
}

type agentSessionDetail struct {
	agentSessionSummary
	PromptAvailable   bool   `json:"prompt_available"`
	ResponseAvailable bool   `json:"response_available"`
	Prompt            string `json:"prompt,omitempty"`
	Response          string `json:"response,omitempty"`
	// Truncated is true when either Prompt or Response was clamped by
	// evidence.ClampOutput on egress. FEAT-022 §10 / Stage E1.
	Truncated bool `json:"truncated"`
	// OriginalBytes is the sum of the original byte sizes of the inlined
	// Prompt and Response bodies before clamping. FEAT-022 §10 / Stage E1.
	OriginalBytes int `json:"original_bytes"`
}

func (s *Server) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.loadSessionsFor(s.workingDirForRequest(r))
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Apply filters
	harness := r.URL.Query().Get("harness")
	since := r.URL.Query().Get("since")
	var sinceTime time.Time
	if since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			sinceTime = t
		}
	}

	if harness != "" || !sinceTime.IsZero() {
		var filtered []agent.SessionEntry
		for _, s := range sessions {
			if harness != "" && s.Harness != harness {
				continue
			}
			if !sinceTime.IsZero() && s.Timestamp.Before(sinceTime) {
				continue
			}
			filtered = append(filtered, s)
		}
		sessions = filtered
	}

	if sessions == nil {
		sessions = []agent.SessionEntry{}
	}

	// Return most recent first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp.After(sessions[j].Timestamp)
	})

	summaries := make([]agentSessionSummary, 0, len(sessions))
	for _, sess := range sessions {
		summaries = append(summaries, summarizeAgentSession(sess))
	}

	writeJSON(w, http.StatusOK, summaries)
}

func (s *Server) handleAgentSessionDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	workDir := s.workingDirForRequest(r)
	logDir := agent.SessionLogDirForWorkDir(workDir)
	idx, ok, err := agent.FindSessionIndex(logDir, id)
	if err == nil && ok {
		sess := agent.SessionIndexEntryToLegacy(idx)
		bodies := agent.LoadSessionBodies(workDir, idx)
		sess.Prompt = bodies.Prompt
		sess.Response = bodies.Response
		sess.Stderr = bodies.Stderr
		writeJSON(w, http.StatusOK, detailAgentSession(sess))
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
}

func (s *Server) loadSessions() ([]agent.SessionEntry, error) {
	return s.loadSessionsFor(s.WorkingDir)
}

func (s *Server) loadSessionsFor(workDir string) ([]agent.SessionEntry, error) {
	logDir := agent.SessionLogDirForWorkDir(workDir)
	entries, err := agent.ReadSessionIndex(logDir, agent.SessionIndexQuery{})
	if err != nil {
		return nil, err
	}
	sessions := make([]agent.SessionEntry, 0, len(entries))
	for _, entry := range entries {
		sessions = append(sessions, agent.SessionIndexEntryToLegacy(entry))
	}
	return sessions, nil
}

func summarizeAgentSession(sess agent.SessionEntry) agentSessionSummary {
	return agentSessionSummary{
		ID:              sess.ID,
		Timestamp:       sess.Timestamp,
		Harness:         sess.Harness,
		Surface:         sess.Surface,
		CanonicalTarget: sess.CanonicalTarget,
		Model:           sess.Model,
		PromptLen:       sess.PromptLen,
		PromptSource:    sess.PromptSource,
		Correlation:     sess.Correlation,
		NativeSessionID: sess.NativeSessionID,
		NativeLogRef:    sess.NativeLogRef,
		TraceID:         sess.TraceID,
		SpanID:          sess.SpanID,
		Stderr:          sess.Stderr,
		Tokens:          sess.Tokens,
		InputTokens:     sess.InputTokens,
		OutputTokens:    sess.OutputTokens,
		CostUSD:         sess.CostUSD,
		DurationMS:      sess.Duration,
		ExitCode:        sess.ExitCode,
		Error:           sess.Error,
		TotalTokens:     sess.TotalTokens,
		BaseRev:         sess.BaseRev,
		ResultRev:       sess.ResultRev,
	}
}

func detailAgentSession(sess agent.SessionEntry) agentSessionDetail {
	detail := agentSessionDetail{
		agentSessionSummary: summarizeAgentSession(sess),
		PromptAvailable:     sess.Prompt != "",
		ResponseAvailable:   sess.Response != "",
	}
	if detail.PromptAvailable {
		clamped, truncated, originalBytes := evidence.ClampOutput(sess.Prompt, serverInlineCapBytes)
		detail.Prompt = clamped
		detail.OriginalBytes += originalBytes
		if truncated {
			detail.Truncated = true
		}
	}
	if detail.ResponseAvailable {
		clamped, truncated, originalBytes := evidence.ClampOutput(sess.Response, serverInlineCapBytes)
		detail.Response = clamped
		detail.OriginalBytes += originalBytes
		if truncated {
			detail.Truncated = true
		}
	}
	return detail
}

// --- Process Metrics Endpoints ---

func (s *Server) handleMetricsSummary(w http.ResponseWriter, r *http.Request) {
	query, err := s.metricsQueryFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	report, err := processmetrics.New(s.workingDirForRequest(r)).Summary(query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleMetricsCost(w http.ResponseWriter, r *http.Request) {
	query, err := s.metricsQueryFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	query.BeadID = strings.TrimSpace(r.URL.Query().Get("bead"))
	query.FeatureID = strings.TrimSpace(r.URL.Query().Get("feature"))
	if query.BeadID != "" && query.FeatureID != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "use either bead or feature, not both"})
		return
	}
	report, err := processmetrics.New(s.workingDirForRequest(r)).Cost(query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleMetricsCycleTime(w http.ResponseWriter, r *http.Request) {
	query, err := s.metricsQueryFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	report, err := processmetrics.New(s.workingDirForRequest(r)).CycleTime(query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleMetricsRework(w http.ResponseWriter, r *http.Request) {
	query, err := s.metricsQueryFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	report, err := processmetrics.New(s.workingDirForRequest(r)).Rework(query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) metricsQueryFromRequest(r *http.Request) (processmetrics.Query, error) {
	rawSince := strings.TrimSpace(r.URL.Query().Get("since"))
	since, err := processmetrics.ParseSince(rawSince)
	if err != nil {
		return processmetrics.Query{}, err
	}
	return processmetrics.Query{
		Since:    since,
		HasSince: rawSince != "",
	}, nil
}

func (s *Server) handleMetricHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "metric id is required"})
		return
	}
	store := metric.NewStore(s.workingDirForRequest(r))
	history, err := store.History(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleMetricTrend(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "metric id is required"})
		return
	}
	store := metric.NewStore(s.workingDirForRequest(r))
	trend, err := store.Trend(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, trend)
}

// --- MCP Endpoint (JSON-RPC 2.0 over Streamable HTTP) ---

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// beadRoutingEvent is the parsed body of a kind:routing bead event.
type beadRoutingEvent struct {
	ResolvedProvider string    `json:"resolved_provider"`
	ResolvedModel    string    `json:"resolved_model,omitempty"`
	RouteReason      string    `json:"route_reason,omitempty"`
	FallbackChain    []string  `json:"fallback_chain"`
	BaseURL          string    `json:"base_url,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	Summary          string    `json:"summary,omitempty"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// Truncated is true when Text was clamped by evidence.ClampOutput.
	// Always present (false when the body fit within the cap). FEAT-022 §10.
	Truncated bool `json:"truncated"`
	// OriginalBytes is len(originalText) before clamping. Always present
	// (equals len(Text) when not truncated). FEAT-022 §10.
	OriginalBytes int `json:"original_bytes"`
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}

	var resp jsonRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "ddx-server",
				"version": "0.1.0",
			},
		}
	case "tools/list":
		resp.Result = map[string]any{
			"tools": s.mcpTools(),
		}
	case "tools/call":
		resp.Result = s.mcpCallTool(req.Params, r)
	case "notifications/initialized":
		resp.Result = map[string]any{}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found"}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) mcpTools() []mcpTool {
	// projectProp is the schema fragment for the optional "project" argument
	// accepted by every project-local tool. Omitted when exactly one project is
	// registered (singleton compat). Required when more than one project is
	// registered — otherwise the tool returns a disambiguation error.
	projectProp := map[string]any{
		"type":        "string",
		"description": "Project ID (proj-...) or path. Optional when exactly one project is registered.",
	}
	return []mcpTool{
		{
			Name:        "ddx_list_documents",
			Description: "List documents in the DDx library",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_read_document",
			Description: "Read the content of a document from the DDx library",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Document path relative to library root"},
					"project": projectProp,
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "ddx_search",
			Description: "Full-text search across library documents",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":   map[string]any{"type": "string", "description": "Search query"},
					"project": projectProp,
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "ddx_list_personas",
			Description: "List all available personas",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_resolve_persona",
			Description: "Resolve the persona bound to a role",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role":    map[string]any{"type": "string", "description": "Role name to resolve"},
					"project": projectProp,
				},
				"required": []string{"role"},
			},
		},
		{
			Name:        "ddx_list_beads",
			Description: "List work items (beads) with optional filters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":  map[string]any{"type": "string", "description": "Filter by status (open, in_progress, closed)"},
					"label":   map[string]any{"type": "string", "description": "Filter by label"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_show_bead",
			Description: "Show details of a specific bead",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Bead ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_bead_ready",
			Description: "List ready beads (open with all dependencies closed)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_bead_status",
			Description: "Get bead summary counts by status",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_bead_blocked",
			Description: "List blocked beads (open beads with unmet dependencies)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_bead_dep_tree",
			Description: "Get the dependency tree for a specific bead",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Bead ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_bead_evidence",
			Description: "List all execution evidence events recorded for a bead",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Bead ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_bead_cooldown",
			Description: "Show the execute-loop cooldown fields for a bead (retry-after, last-status, last-detail)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Bead ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_bead_routing",
			Description: "Show routing decisions recorded for a bead (provider, model, reason, fallback chain)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Bead ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_doc_graph",
			Description: "Get the full document dependency graph",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_doc_stale",
			Description: "List stale documents",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_doc_show",
			Description: "Show document metadata and staleness",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_doc_deps",
			Description: "Get upstream dependencies of a document",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_doc_dependents",
			Description: "Get documents that depend on a given document (reverse dependency direction)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_agent_sessions",
			Description: "List recent agent sessions",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"harness": map[string]any{"type": "string", "description": "Filter by harness name"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_provider_list",
			Description: "List all configured provider harnesses with routing availability, auth state, quota/headroom, and signal freshness (host+user global, not project-scoped)",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_provider_show",
			Description: "Get full routing signal snapshot for one provider harness (FEAT-014 read model)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"harness": map[string]any{"type": "string", "description": "Harness name (e.g. claude, codex, gemini)"},
				},
				"required": []string{"harness"},
			},
		},
		{
			Name:        "ddx_agent_models",
			Description: "List models for a configured provider (or all providers)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"provider": map[string]any{"type": "string", "description": "Provider name (default: configured default)"},
					"all":      map[string]any{"type": "boolean", "description": "If true, return models for every configured provider"},
					"project":  projectProp,
				},
			},
		},
		{
			Name:        "ddx_agent_catalog",
			Description: "Show the current model catalog (tier→surface→model assignments and model metadata)",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_agent_capabilities",
			Description: "Show model and reasoning-level capabilities for a harness",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"harness": map[string]any{"type": "string", "description": "Harness name (default: first available)"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_bead_create",
			Description: "Create a new bead (work item)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]any{"type": "string", "description": "Bead title"},
					"type":        map[string]any{"type": "string", "description": "Issue type (task, bug, epic, chore)"},
					"priority":    map[string]any{"type": "integer", "description": "Priority (0=highest, 4=lowest)"},
					"labels":      map[string]any{"type": "string", "description": "Comma-separated labels"},
					"description": map[string]any{"type": "string", "description": "Description"},
					"acceptance":  map[string]any{"type": "string", "description": "Acceptance criteria"},
					"project":     projectProp,
				},
				"required": []string{"title"},
			},
		},
		{
			Name:        "ddx_bead_update",
			Description: "Update fields of an existing bead",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Bead ID"},
					"status":      map[string]any{"type": "string", "description": "New status (open, in_progress, closed)"},
					"labels":      map[string]any{"type": "string", "description": "Comma-separated labels (replaces existing)"},
					"description": map[string]any{"type": "string", "description": "New description"},
					"acceptance":  map[string]any{"type": "string", "description": "New acceptance criteria"},
					"project":     projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_bead_claim",
			Description: "Claim a bead for the current session (sets status to in_progress)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":       map[string]any{"type": "string", "description": "Bead ID"},
					"assignee": map[string]any{"type": "string", "description": "Assignee name"},
					"project":  projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_exec_definitions",
			Description: "List execution definitions with optional artifact filter",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artifact": map[string]any{"type": "string", "description": "Filter by artifact ID"},
					"project":  projectProp,
				},
			},
		},
		{
			Name:        "ddx_exec_show",
			Description: "Show a specific execution definition",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Definition ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_exec_history",
			Description: "List execution runs with optional artifact and definition filters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artifact":   map[string]any{"type": "string", "description": "Filter by artifact ID"},
					"definition": map[string]any{"type": "string", "description": "Filter by definition ID"},
					"project":    projectProp,
				},
			},
		},
		{
			Name:        "ddx_exec_dispatch",
			Description: "Dispatch an execution run by definition ID (localhost-only)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Execution definition ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_exec_run",
			Description: "Get the result of a single execution run by run ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Execution run ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_exec_run_log",
			Description: "Get the stdout and stderr log of a single execution run by run ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Execution run ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_agent_dispatch",
			Description: "Dispatch an agent invocation (localhost-only)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"harness": map[string]any{"type": "string", "description": "Agent harness name (codex, claude, gemini)"},
					"prompt":  map[string]any{"type": "string", "description": "Prompt text"},
					"model":   map[string]any{"type": "string", "description": "Model override"},
					"effort":  map[string]any{"type": "string", "description": "Effort/reasoning level"},
					"project": projectProp,
				},
				"required": []string{"harness", "prompt"},
			},
		},
		{
			Name:        "ddx_doc_changed",
			Description: "List artifacts changed since a git ref",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since":   map[string]any{"type": "string", "description": "Git ref to compare from (default: HEAD~5)"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_doc_write",
			Description: "Write content to a document by artifact ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document artifact ID"},
					"content": map[string]any{"type": "string", "description": "New document content"},
					"project": projectProp,
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "ddx_doc_history",
			Description: "Get git commit history for a document",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document artifact ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_doc_diff",
			Description: "Get git diff for a document",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document artifact ID"},
					"ref":     map[string]any{"type": "string", "description": "Git ref to diff against (default: working copy vs HEAD)"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_metrics_summary",
			Description: "Process metrics summary: throughput, lead time, and rework rate",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since":   map[string]any{"type": "string", "description": "Time window: today, Nd (e.g. 7d), YYYY-MM-DD, or RFC3339"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_metrics_cost",
			Description: "Token cost breakdown by bead or feature",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since":   map[string]any{"type": "string", "description": "Time window: today, Nd (e.g. 7d), YYYY-MM-DD, or RFC3339"},
					"bead":    map[string]any{"type": "string", "description": "Filter to a specific bead ID (mutually exclusive with feature)"},
					"feature": map[string]any{"type": "string", "description": "Filter to a specific feature ID (mutually exclusive with bead)"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_metrics_cycle_time",
			Description: "Cycle-time distribution: time from bead open to close",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since":   map[string]any{"type": "string", "description": "Time window: today, Nd (e.g. 7d), YYYY-MM-DD, or RFC3339"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_metrics_rework",
			Description: "Rework metrics: beads reopened or retried after closure",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since":   map[string]any{"type": "string", "description": "Time window: today, Nd (e.g. 7d), YYYY-MM-DD, or RFC3339"},
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_metric_history",
			Description: "Observation history for a specific metric ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Metric artifact ID (MET-...)"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_metric_trend",
			Description: "Trend summary (min, max, average, latest) for a specific metric ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Metric artifact ID (MET-...)"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_list_projects",
			Description: "List projects registered with this ddx-server node",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_show_project",
			Description: "Show a registered project by ID or path",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"type": "string", "description": "Project ID (proj-...)"},
					"path": map[string]any{"type": "string", "description": "Project path"},
				},
			},
		},
		{
			Name:        "ddx_worker_list",
			Description: "List agent workers for the project",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_worker_show",
			Description: "Show details of a specific agent worker",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Worker ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_worker_log",
			Description: "Get stdout/stderr log for a specific agent worker",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Worker ID"},
					"project": projectProp,
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_list_mcp_servers",
			Description: "List MCP servers available in the DDx library registry",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": projectProp,
				},
			},
		},
		{
			Name:        "ddx_list_plugins",
			Description: "List installed DDx plugins",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (s *Server) mcpCallTool(params json.RawMessage, r *http.Request) mcpToolResult {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText("invalid tool call parameters")},
			IsError: true,
		}
	}

	// Tools that operate on project-local data resolve the optional "project"
	// argument first; tools listed in the default branch are host/user global
	// and ignore project scoping entirely.
	project, _ := call.Arguments["project"].(string)

	switch call.Name {
	case "ddx_list_projects":
		return s.mcpListProjects()
	case "ddx_show_project":
		id, _ := call.Arguments["id"].(string)
		path, _ := call.Arguments["path"].(string)
		return s.mcpShowProject(id, path)
	case "ddx_provider_list":
		return s.mcpProviderList()
	case "ddx_provider_show":
		harness, _ := call.Arguments["harness"].(string)
		return s.mcpProviderShow(harness)
	case "ddx_agent_catalog":
		return s.mcpAgentCatalog()
	case "ddx_list_plugins":
		return s.mcpListPlugins()
	}

	// From here on: project-local tools. Resolve the project arg to a working
	// directory before dispatching.
	workingDir, errResult := s.mcpResolveWorkingDir(project)
	if errResult != nil {
		return *errResult
	}

	switch call.Name {
	case "ddx_list_documents":
		return s.mcpListDocuments(workingDir)
	case "ddx_read_document":
		path, _ := call.Arguments["path"].(string)
		return s.mcpReadDocument(workingDir, path)
	case "ddx_search":
		query, _ := call.Arguments["query"].(string)
		return s.mcpSearch(workingDir, query)
	case "ddx_list_personas":
		return s.mcpListPersonas(workingDir)
	case "ddx_resolve_persona":
		role, _ := call.Arguments["role"].(string)
		return s.mcpResolvePersona(workingDir, role)
	case "ddx_list_beads":
		status, _ := call.Arguments["status"].(string)
		label, _ := call.Arguments["label"].(string)
		return s.mcpListBeads(workingDir, status, label)
	case "ddx_show_bead":
		id, _ := call.Arguments["id"].(string)
		return s.mcpShowBead(workingDir, id)
	case "ddx_bead_ready":
		return s.mcpBeadReady(workingDir)
	case "ddx_bead_status":
		return s.mcpBeadStatus(workingDir)
	case "ddx_bead_blocked":
		return s.mcpBeadBlocked(workingDir)
	case "ddx_bead_dep_tree":
		id, _ := call.Arguments["id"].(string)
		return s.mcpBeadDepTree(workingDir, id)
	case "ddx_bead_evidence":
		id, _ := call.Arguments["id"].(string)
		return s.mcpBeadEvidence(workingDir, id)
	case "ddx_bead_cooldown":
		id, _ := call.Arguments["id"].(string)
		return s.mcpBeadCooldown(workingDir, id)
	case "ddx_bead_routing":
		id, _ := call.Arguments["id"].(string)
		return s.mcpBeadRouting(workingDir, id)
	case "ddx_doc_graph":
		return s.mcpDocGraph(workingDir)
	case "ddx_doc_stale":
		return s.mcpDocStale(workingDir)
	case "ddx_doc_show":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocShow(workingDir, id)
	case "ddx_doc_deps":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocDeps(workingDir, id)
	case "ddx_doc_dependents":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocDependents(workingDir, id)
	case "ddx_agent_sessions":
		harness, _ := call.Arguments["harness"].(string)
		return s.mcpAgentSessions(workingDir, harness)
	case "ddx_agent_models":
		providerName, _ := call.Arguments["provider"].(string)
		showAll, _ := call.Arguments["all"].(bool)
		return s.mcpAgentModels(workingDir, providerName, showAll)
	case "ddx_agent_capabilities":
		harness, _ := call.Arguments["harness"].(string)
		return s.mcpAgentCapabilities(workingDir, harness)
	case "ddx_bead_create":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{mcpText("forbidden: write tools require trusted origin")}, IsError: true}
		}
		title, _ := call.Arguments["title"].(string)
		issueType, _ := call.Arguments["type"].(string)
		labelsStr, _ := call.Arguments["labels"].(string)
		description, _ := call.Arguments["description"].(string)
		acceptance, _ := call.Arguments["acceptance"].(string)
		var priority int
		if p, ok := call.Arguments["priority"].(float64); ok {
			priority = int(p)
		}
		return s.mcpBeadCreate(workingDir, title, issueType, priority, labelsStr, description, acceptance)
	case "ddx_bead_update":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{mcpText("forbidden: write tools require trusted origin")}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		status, _ := call.Arguments["status"].(string)
		labelsStr, _ := call.Arguments["labels"].(string)
		description, _ := call.Arguments["description"].(string)
		acceptance, _ := call.Arguments["acceptance"].(string)
		return s.mcpBeadUpdate(workingDir, id, status, labelsStr, description, acceptance)
	case "ddx_bead_claim":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{mcpText("forbidden: write tools require trusted origin")}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		assignee, _ := call.Arguments["assignee"].(string)
		return s.mcpBeadClaim(workingDir, id, assignee)
	case "ddx_exec_definitions":
		artifact, _ := call.Arguments["artifact"].(string)
		return s.mcpExecDefinitions(workingDir, artifact)
	case "ddx_exec_show":
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecShow(workingDir, id)
	case "ddx_exec_history":
		artifact, _ := call.Arguments["artifact"].(string)
		definition, _ := call.Arguments["definition"].(string)
		return s.mcpExecHistory(workingDir, artifact, definition)
	case "ddx_exec_dispatch":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{mcpText("forbidden: dispatch tools require trusted origin")}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecDispatch(workingDir, id)
	case "ddx_exec_run":
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecRun(workingDir, id)
	case "ddx_exec_run_log":
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecRunLog(workingDir, id)
	case "ddx_agent_dispatch":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{mcpText("forbidden: dispatch tools require trusted origin")}, IsError: true}
		}
		harness, _ := call.Arguments["harness"].(string)
		prompt, _ := call.Arguments["prompt"].(string)
		model, _ := call.Arguments["model"].(string)
		effort, _ := call.Arguments["effort"].(string)
		return s.mcpAgentDispatch(workingDir, harness, prompt, model, effort)
	case "ddx_doc_changed":
		since, _ := call.Arguments["since"].(string)
		return s.mcpDocChanged(workingDir, since)
	case "ddx_doc_write":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{mcpText("forbidden: write tools require trusted origin")}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		content, _ := call.Arguments["content"].(string)
		return s.mcpDocWrite(workingDir, id, content)
	case "ddx_doc_history":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocHistory(workingDir, id)
	case "ddx_doc_diff":
		id, _ := call.Arguments["id"].(string)
		ref, _ := call.Arguments["ref"].(string)
		return s.mcpDocDiff(workingDir, id, ref)
	case "ddx_worker_list":
		return s.mcpWorkerList(workingDir)
	case "ddx_worker_show":
		id, _ := call.Arguments["id"].(string)
		return s.mcpWorkerShow(workingDir, id)
	case "ddx_worker_log":
		id, _ := call.Arguments["id"].(string)
		return s.mcpWorkerLog(workingDir, id)
	case "ddx_metrics_summary":
		since, _ := call.Arguments["since"].(string)
		return s.mcpMetricsSummary(workingDir, since)
	case "ddx_metrics_cost":
		since, _ := call.Arguments["since"].(string)
		beadID, _ := call.Arguments["bead"].(string)
		featureID, _ := call.Arguments["feature"].(string)
		return s.mcpMetricsCost(workingDir, since, beadID, featureID)
	case "ddx_metrics_cycle_time":
		since, _ := call.Arguments["since"].(string)
		return s.mcpMetricsCycleTime(workingDir, since)
	case "ddx_metrics_rework":
		since, _ := call.Arguments["since"].(string)
		return s.mcpMetricsRework(workingDir, since)
	case "ddx_metric_history":
		id, _ := call.Arguments["id"].(string)
		return s.mcpMetricHistory(workingDir, id)
	case "ddx_metric_trend":
		id, _ := call.Arguments["id"].(string)
		return s.mcpMetricTrend(workingDir, id)
	case "ddx_list_mcp_servers":
		return s.mcpListMCPServers(workingDir)
	default:
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf("unknown tool: %s", call.Name))},
			IsError: true,
		}
	}
}

// --- MCP Tool Implementations ---

func (s *Server) mcpListDocuments(workingDir string) mcpToolResult {
	libPath := s.libraryPathFor(workingDir)
	if libPath == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}

	type docEntry struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	var docs []docEntry
	categories := []string{"prompts", "templates", "personas", "patterns", "configs", "scripts", "mcp-servers"}
	for _, cat := range categories {
		catDir := filepath.Join(libPath, cat)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			docs = append(docs, docEntry{
				Name: e.Name(),
				Type: cat,
				Path: filepath.Join(cat, e.Name()),
			})
		}
	}
	data, _ := json.Marshal(docs)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpReadDocument(workingDir, path string) mcpToolResult {
	if path == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("path is required")},
			IsError: true,
		}
	}
	libPath := s.libraryPathFor(workingDir)
	if libPath == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("library not configured")},
			IsError: true,
		}
	}
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return mcpToolResult{
			Content: []mcpContent{mcpText("invalid path")},
			IsError: true,
		}
	}
	data, err := os.ReadFile(filepath.Join(libPath, cleaned))
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText("document not found")},
			IsError: true,
		}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpSearch(workingDir, query string) mcpToolResult {
	if query == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("query is required")},
			IsError: true,
		}
	}

	libPath := s.libraryPathFor(workingDir)
	if libPath == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}

	type searchResult struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Name string `json:"name"`
	}

	var results []searchResult
	queryLower := strings.ToLower(query)
	categories := []string{"prompts", "templates", "personas", "patterns", "configs", "scripts", "mcp-servers"}

	for _, cat := range categories {
		catDir := filepath.Join(libPath, cat)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			relPath := filepath.Join(cat, e.Name())
			nameLower := strings.ToLower(e.Name())
			if strings.Contains(nameLower, queryLower) {
				results = append(results, searchResult{Path: relPath, Type: cat, Name: e.Name()})
				continue
			}
			fullPath := filepath.Join(libPath, relPath)
			if data, err := os.ReadFile(fullPath); err == nil {
				if strings.Contains(strings.ToLower(string(data)), queryLower) {
					results = append(results, searchResult{Path: relPath, Type: cat, Name: e.Name()})
				}
			}
		}
	}

	data, _ := json.Marshal(results)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpListPersonas(workingDir string) mcpToolResult {
	loader := persona.NewPersonaLoader(workingDir)
	personas, err := loader.ListPersonas()
	if err != nil {
		data, _ := json.Marshal([]any{})
		return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
	}

	result := make([]map[string]any, 0, len(personas))
	for _, p := range personas {
		result = append(result, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"roles":       p.Roles,
			"tags":        p.Tags,
		})
	}
	data, _ := json.Marshal(result)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpResolvePersona(workingDir, role string) mcpToolResult {
	if role == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("role is required")},
			IsError: true,
		}
	}

	bm := persona.NewBindingManagerWithPath(filepath.Join(workingDir, ".ddx.yml"))
	personaName, err := bm.GetBinding(role)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf("no persona bound to role: %s", role))},
			IsError: true,
		}
	}

	loader := persona.NewPersonaLoader(workingDir)
	p, err := loader.LoadPersona(personaName)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf("persona not found: %s", personaName))},
			IsError: true,
		}
	}

	result := map[string]any{
		"role":        role,
		"persona":     p.Name,
		"description": p.Description,
		"content":     p.Content,
	}
	data, _ := json.Marshal(result)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpListBeads(workingDir, status, label string) mcpToolResult {
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	beads, err := store.List(status, label, nil)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	if beads == nil {
		beads = []bead.Bead{}
	}
	data, _ := json.Marshal(beads)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpShowBead(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	b, err := store.Get(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText("bead not found")},
			IsError: true,
		}
	}
	data, _ := json.Marshal(b)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpListProjects() mcpToolResult {
	projects := s.state.GetProjects()
	if projects == nil {
		projects = []ProjectEntry{}
	}
	data, _ := json.Marshal(projects)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpShowProject(id, path string) mcpToolResult {
	if id == "" && path == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id or path is required")},
			IsError: true,
		}
	}
	var (
		entry ProjectEntry
		ok    bool
	)
	if id != "" {
		entry, ok = s.state.GetProjectByID(id)
	}
	if !ok && path != "" {
		entry, ok = s.state.GetProjectByPath(path)
	}
	if !ok {
		return mcpToolResult{
			Content: []mcpContent{mcpText("project not found")},
			IsError: true,
		}
	}
	data, _ := json.Marshal(entry)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadReady(workingDir string) mcpToolResult {
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	ready, err := store.Ready()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	if ready == nil {
		ready = []bead.Bead{}
	}
	data, _ := json.Marshal(ready)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadStatus(workingDir string) mcpToolResult {
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	counts, err := store.Status()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	data, _ := json.Marshal(counts)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadBlocked(workingDir string) mcpToolResult {
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	blocked, err := store.Blocked()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	if blocked == nil {
		blocked = []bead.Bead{}
	}
	data, _ := json.Marshal(blocked)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadDepTree(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText(`{"error":"id required"}`)},
			IsError: true,
		}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	tree, err := store.DepTree(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	data, _ := json.Marshal(map[string]string{"id": id, "tree": tree})
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocGraph(workingDir string) mcpToolResult {
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}

	type graphNode struct {
		ID         string   `json:"id"`
		Path       string   `json:"path"`
		DependsOn  []string `json:"depends_on,omitempty"`
		Dependents []string `json:"dependents,omitempty"`
	}
	nodes := make([]graphNode, 0, len(graph.Documents))
	for _, doc := range graph.Documents {
		nodes = append(nodes, graphNode{
			ID:         doc.ID,
			Path:       doc.Path,
			DependsOn:  doc.DependsOn,
			Dependents: doc.Dependents,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	data, _ := json.Marshal(nodes)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocStale(workingDir string) mcpToolResult {
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	stale := graph.StaleDocs()
	if stale == nil {
		stale = []docgraph.StaleReason{}
	}
	data, _ := json.Marshal(stale)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocShow(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{
			Content: []mcpContent{mcpText("document not found")},
			IsError: true,
		}
	}
	staleReason, isStale := graph.StaleReasonForID(id)
	resp := map[string]any{
		"id":         doc.ID,
		"path":       doc.Path,
		"title":      doc.Title,
		"depends_on": doc.DependsOn,
		"dependents": doc.Dependents,
		"is_stale":   isStale,
	}
	if isStale {
		resp["stale_reasons"] = staleReason.Reasons
	}
	data, _ := json.Marshal(resp)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocDeps(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	deps, err := graph.Dependencies(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(err.Error())},
			IsError: true,
		}
	}
	data, _ := json.Marshal(deps)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocDependents(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	dependents, err := graph.DependentIDs(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(err.Error())},
			IsError: true,
		}
	}
	data, _ := json.Marshal(dependents)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpAgentSessions(workingDir, harness string) mcpToolResult {
	sessions, err := s.loadSessionsFor(workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	if harness != "" {
		var filtered []agent.SessionEntry
		for _, sess := range sessions {
			if sess.Harness == harness {
				filtered = append(filtered, sess)
			}
		}
		sessions = filtered
	}
	if sessions == nil {
		sessions = []agent.SessionEntry{}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp.After(sessions[j].Timestamp)
	})
	data, _ := json.Marshal(sessions)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadCreate(workingDir, title, issueType string, priority int, labelsStr, description, acceptance string) mcpToolResult {
	if title == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("title is required")},
			IsError: true,
		}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	b := &bead.Bead{
		Title:       title,
		IssueType:   issueType,
		Priority:    priority,
		Description: description,
		Acceptance:  acceptance,
	}
	if labelsStr != "" {
		b.Labels = strings.Split(labelsStr, ",")
	}
	if err := store.Create(b); err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(err.Error())},
			IsError: true,
		}
	}
	data, _ := json.Marshal(b)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadUpdate(workingDir, id, status, labelsStr, description, acceptance string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	err := store.Update(id, func(b *bead.Bead) {
		if status != "" {
			b.Status = status
		}
		if labelsStr != "" {
			b.Labels = strings.Split(labelsStr, ",")
		}
		if description != "" {
			b.Description = description
		}
		if acceptance != "" {
			b.Acceptance = acceptance
		}
	})
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(err.Error())},
			IsError: true,
		}
	}
	updated, _ := store.Get(id)
	data, _ := json.Marshal(updated)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadClaim(workingDir, id, assignee string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	if err := store.ClaimWithOptions(id, assignee, "", ""); err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(err.Error())},
			IsError: true,
		}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(fmt.Sprintf(`{"id":"%s","status":"claimed"}`, id))}}
}

func (s *Server) mcpExecDefinitions(workingDir, artifactID string) mcpToolResult {
	store := ddxexec.NewStore(workingDir)
	defs, err := store.ListDefinitions(artifactID)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	data, _ := json.Marshal(defs)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpExecShow(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText("id is required")},
			IsError: true,
		}
	}
	store := ddxexec.NewStore(workingDir)
	def, err := store.ShowDefinition(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(err.Error())},
			IsError: true,
		}
	}
	data, _ := json.Marshal(def)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpExecHistory(workingDir, artifactID, definitionID string) mcpToolResult {
	store := ddxexec.NewStore(workingDir)
	runs, err := store.History(artifactID, definitionID)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	data, _ := json.Marshal(runs)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocWrite(workingDir, id, content string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	if content == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("content is required")}, IsError: true}
	}
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{Content: []mcpContent{mcpText("document not found")}, IsError: true}
	}
	// doc.Path is relative to the docgraph root; resolve against workingDir
	// before touching the file system.
	fullPath := doc.Path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(workingDir, fullPath)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	committed := false
	var acCfg internalgit.AutoCommitConfig
	if cfg, cfgErr := config.LoadWithWorkingDir(workingDir); cfgErr == nil && cfg.Git != nil {
		acCfg.AutoCommit = cfg.Git.AutoCommit
		acCfg.CommitPrefix = cfg.Git.CommitPrefix
	}
	if acCfg.AutoCommit == "always" {
		if _, acErr := internalgit.AutoCommit(fullPath, id, "write document", acCfg); acErr == nil {
			committed = true
		}
	}
	data, _ := json.Marshal(map[string]any{"status": "ok", "path": doc.Path, "committed": committed})
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocHistory(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{Content: []mcpContent{mcpText("document not found")}, IsError: true}
	}
	logCmd := internalgit.Command(context.Background(), workingDir, "log", "--follow", "--format=%H\t%ai\t%an\t%s", "--", doc.Path)
	out, gitErr := logCmd.Output()
	if gitErr != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("git log failed")}, IsError: true}
	}
	type commitEntry struct {
		Hash    string `json:"hash"`
		Date    string `json:"date"`
		Author  string `json:"author"`
		Message string `json:"message"`
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	entries := make([]commitEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		hash := parts[0]
		if len(hash) > 7 {
			hash = hash[:7]
		}
		date := parts[1]
		if len(date) > 10 {
			date = date[:10]
		}
		entries = append(entries, commitEntry{Hash: hash, Date: date, Author: parts[2], Message: parts[3]})
	}
	data, _ := json.Marshal(entries)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocDiff(workingDir, id, ref string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{Content: []mcpContent{mcpText("document not found")}, IsError: true}
	}
	var gitArgs []string
	if ref != "" {
		gitArgs = []string{"diff", ref, "--", doc.Path}
	} else {
		gitArgs = []string{"diff", "--", doc.Path}
	}
	mcpDiffCmd := internalgit.Command(context.Background(), workingDir, gitArgs...)
	out, gitErr := mcpDiffCmd.Output()
	if gitErr != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("git diff failed")}, IsError: true}
	}
	data, _ := json.Marshal(map[string]string{"diff": string(out)})
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpExecRun(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := ddxexec.NewStore(workingDir)
	result, err := store.Result(id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(result)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpExecRunLog(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := ddxexec.NewStore(workingDir)
	stdout, stderr, err := store.Log(id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(map[string]string{"stdout": stdout, "stderr": stderr})
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpExecDispatch(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := ddxexec.NewStore(workingDir)
	record, err := store.Run(context.Background(), id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(record)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpAgentDispatch(workingDir, harness, prompt, model, effort string) mcpToolResult {
	if harness == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("harness is required")}, IsError: true}
	}
	if prompt == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("prompt is required")}, IsError: true}
	}
	rcfg, _ := config.LoadAndResolve(workingDir, config.CLIOverrides{
		Harness: harness,
		Model:   model,
		Effort:  effort,
	})
	runtime := agent.AgentRunRuntime{
		Prompt:  prompt,
		WorkDir: workingDir,
	}
	result, err := agent.RunWithConfigViaService(context.Background(), workingDir, rcfg, runtime)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(result)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpDocChanged(workingDir, since string) mcpToolResult {
	if since == "" {
		since = "HEAD~5"
	}
	diffCmd := internalgit.Command(context.Background(), workingDir, "diff", "--name-status", since, "HEAD")
	out, gitErr := diffCmd.Output()
	if gitErr != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("git diff failed")}, IsError: true}
	}

	graph, err := docgraph.BuildGraphWithConfig(workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}

	rootCmd := internalgit.Command(context.Background(), workingDir, "rev-parse", "--show-toplevel")
	rootOut, rootErr := rootCmd.Output()
	if rootErr != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("could not determine git root")}, IsError: true}
	}
	repoRoot := strings.TrimRight(string(rootOut), "\n")

	type changedEntry struct {
		ID         string `json:"id"`
		Path       string `json:"path"`
		ChangeType string `json:"change_type"`
	}

	var entries []changedEntry
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		statusCode := fields[0]
		relPath := fields[len(fields)-1]
		if !strings.HasSuffix(relPath, ".md") {
			continue
		}

		absPath := filepath.Join(repoRoot, relPath)
		cleanPath := filepath.Clean(absPath)
		graphKey := cleanPath
		if rel, relErr := filepath.Rel(graph.RootDir, cleanPath); relErr == nil {
			graphKey = rel
		}

		var changeType string
		switch {
		case strings.HasPrefix(statusCode, "A"):
			changeType = "added"
		case strings.HasPrefix(statusCode, "D"):
			changeType = "deleted"
		default:
			changeType = "modified"
		}

		if id, ok := graph.PathToID[graphKey]; ok {
			entries = append(entries, changedEntry{ID: id, Path: relPath, ChangeType: changeType})
		}
	}

	data, _ := json.Marshal(entries)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

// --- Helpers ---

func (s *Server) libraryPath() string {
	return s.libraryPathFor(s.WorkingDir)
}

// libraryPathFor resolves the library path rooted at workingDir.
func (s *Server) libraryPathFor(workingDir string) string {
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return ""
	}
	if cfg.Library == nil || cfg.Library.Path == "" {
		return ""
	}
	p := cfg.Library.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(workingDir, p)
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func (s *Server) beadStore() *bead.Store {
	return bead.NewStore(filepath.Join(s.WorkingDir, ".ddx"))
}

func (s *Server) buildDocGraph() (*docgraph.Graph, error) {
	return docgraph.BuildGraphWithConfig(s.WorkingDir)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// --- GraphQL Endpoints ---

func (s *Server) handleGraphQLQuery(w http.ResponseWriter, r *http.Request) {
	// Gate /graphql on the same localhost auth that protects every /api/*
	// handler. Without this, operators running `ddx server --addr 0.0.0.0`
	// ship an unauthenticated GraphQL endpoint serving bead data, worker
	// logs, mutations, and subscriptions. See opus final-gate review of
	// ddx-02d6142d and the scope line "isTrusted() is still the gatekeeper;
	// no auth bypass introduced".
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "graphql is localhost-only"})
		return
	}
	s.serveGraphQL(w, r, s.WorkingDir)
}

// handleGraphQLQueryScoped serves /api/projects/{project}/graphql. The
// projectScoped middleware has already resolved {project} into the request
// context (or written 404 if unknown). LAYER 1 of the GraphQL multi-project
// fix: per-request resolver reconstruction with the resolved project's
// WorkingDir, so DocumentByPath and friends do not leak across projects.
// Closes ddx-4c51d33e.
func (s *Server) handleGraphQLQueryScoped(w http.ResponseWriter, r *http.Request) {
	dir := s.workingDirForRequest(r)
	s.serveGraphQL(w, r, dir)
}

// serveGraphQL serves an HTTP GraphQL request, injecting the per-request
// workingDir into context so the singleton resolver can read it via
// ddxgraphql.WorkingDirFromContext. LAYER 2 of the GraphQL multi-project
// fix (ddx-055e8d32): the gqlgen handler and resolver are built ONCE
// (lazily on first request) and reused across both /graphql and the scoped
// /api/projects/{project}/graphql route. Per-request reconstruction
// (LAYER 1) is no longer needed because resolver methods read WorkingDir
// from the request context rather than a struct field.
func (s *Server) serveGraphQL(w http.ResponseWriter, r *http.Request, workingDir string) {
	gqlServer := s.graphqlHandler()
	// Plumb the originating *http.Request into the resolver context so
	// resolvers (e.g. operatorPromptSubmit) can validate the CSRF header
	// and capture the originating identity from headers tsnetMiddleware
	// injects (X-Tailscale-User / X-Tailscale-Node). Also inject the
	// per-request workingDir so resolvers reading WorkingDir do so under
	// the scoped project (LAYER 2 / ddx-055e8d32) rather than the
	// resolver's startup default.
	ctx := r.Context()
	ctx = ddxgraphql.WithHTTPRequest(ctx, r)
	ctx = ddxgraphql.WithWorkingDir(ctx, workingDir)
	gqlServer.ServeHTTP(w, r.WithContext(ctx))
}

// graphqlHandler lazily builds and caches the gqlgen handler used by both
// /graphql and the scoped /api/projects/{project}/graphql route. The
// resolver's WorkingDir field is set to s.WorkingDir as the FALLBACK
// default; per-request scoping flows through context via WithWorkingDir
// (see serveGraphQL).
func (s *Server) graphqlHandler() http.Handler {
	s.gqlOnce.Do(func() {
		var fedProvider ddxgraphql.FederationProvider
		if s.hub != nil {
			fedProvider = newHubFederationProvider(s)
		}
		gqlServer := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
			Resolvers: &ddxgraphql.Resolver{
				State:                              s.state,
				WorkingDir:                         s.WorkingDir,
				Workers:                            s.workers,
				BeadBus:                            s.beadHub,
				Actions:                            &workerDispatchAdapter{manager: s.workers},
				ExecLogs:                           &execLogAdapter{},
				CoordMetrics:                       &coordMetricsAdapter{reg: s.workers.LandCoordinators},
				CSRFTokens:                         s.csrfTokens,
				OperatorPromptIdempotency:          s.operatorPromptIdempotency,
				OperatorPromptAutoApproveAllowlist: s.operatorPromptAutoApproveAllowlist,
				PromptCapBytes:                     serverPromptCapBytes,
				BuildSHA:                           serverBuildSHA(),
				NodeID:                             s.state.Node.ID,
				ExecuteLoopWaker:                   s.workers,
				Federation:                         fedProvider,
				ReportedWorkers:                    s.reportedWorkers,
			},
			Directives: ddxgraphql.DirectiveRoot{},
		}))
		gqlServer.AddTransport(transport.POST{})
		gqlServer.AddTransport(transport.GET{})
		gqlServer.AddTransport(transport.Websocket{
			Upgrader: websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool { return true },
			},
			KeepAlivePingInterval: 30 * time.Second,
		})
		s.gqlHandler = gqlServer
	})
	return s.gqlHandler
}

func (s *Server) handleGraphiQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Serve GraphiQL HTML page with inline GraphQL endpoint reference
	graphiqlHTML := `<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>GraphiQL - DDx</title>
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graphiql@0.18.0/graphiql.min.css" />
	<script src="https://cdn.jsdelivr.net/npm/subscriptions-transport-ws@0.11.0/browser/client.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/graphiql@3/graphiql.min.js"></script>
</head>
<body>
	<div id="graphiql">Loading...</div>
	<script>
		var fetcher = function (fetchParams) {
			return fetch("/graphql", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(fetchParams),
			}).then(function (response) {
				return response.json();
			});
		};
		var graphiql = GraphiQL.createGraphiQL({ fetcher: fetcher });
		graphiql.renderInto(document.getElementById("graphiql"));
	</script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(graphiqlHTML))
}

func (s *Server) mcpWorkerManager(workingDir string) *WorkerManager {
	if workingDir == s.WorkingDir {
		return s.workers
	}
	return NewWorkerManager(workingDir)
}

func (s *Server) mcpWorkerList(workingDir string) mcpToolResult {
	m := s.mcpWorkerManager(workingDir)
	recs, err := m.List()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
	}
	if recs == nil {
		recs = []WorkerRecord{}
	}
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].StartedAt.After(recs[j].StartedAt)
	})
	data, _ := json.Marshal(recs)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpWorkerShow(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText(`{"error":"id required"}`)},
			IsError: true,
		}
	}
	m := s.mcpWorkerManager(workingDir)
	record, err := m.Show(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	if record.ProjectRoot != "" {
		metrics := s.workers.LandCoordinators.MetricsFor(record.ProjectRoot)
		if metrics != nil {
			record.LandSummary = metrics
		}
	}
	data, _ := json.Marshal(record)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpWorkerLog(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{mcpText(`{"error":"id required"}`)},
			IsError: true,
		}
	}
	m := s.mcpWorkerManager(workingDir)
	stdout, stderr, err := m.Logs(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText(fmt.Sprintf(`{"error":"%s"}`, err.Error()))},
			IsError: true,
		}
	}
	data, _ := json.Marshal(map[string]string{"stdout": stdout, "stderr": stderr})
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpMetricsSummary(workingDir, since string) mcpToolResult {
	q, err := processmetrics.ParseSince(since)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	query := processmetrics.Query{Since: q, HasSince: since != ""}
	report, err := processmetrics.New(workingDir).Summary(query)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(report)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpMetricsCost(workingDir, since, beadID, featureID string) mcpToolResult {
	if beadID != "" && featureID != "" {
		return mcpToolResult{Content: []mcpContent{mcpText("use either bead or feature, not both")}, IsError: true}
	}
	q, err := processmetrics.ParseSince(since)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	query := processmetrics.Query{Since: q, HasSince: since != "", BeadID: beadID, FeatureID: featureID}
	report, err := processmetrics.New(workingDir).Cost(query)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(report)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpMetricsCycleTime(workingDir, since string) mcpToolResult {
	q, err := processmetrics.ParseSince(since)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	query := processmetrics.Query{Since: q, HasSince: since != ""}
	report, err := processmetrics.New(workingDir).CycleTime(query)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(report)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpMetricsRework(workingDir, since string) mcpToolResult {
	q, err := processmetrics.ParseSince(since)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	query := processmetrics.Query{Since: q, HasSince: since != ""}
	report, err := processmetrics.New(workingDir).Rework(query)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(report)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpMetricHistory(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := metric.NewStore(workingDir)
	history, err := store.History(id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(history)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpMetricTrend(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := metric.NewStore(workingDir)
	trend, err := store.Trend(id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, _ := json.Marshal(trend)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadEvidence(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	events, err := store.Events(id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("bead not found")}, IsError: true}
	}
	data, _ := json.Marshal(events)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadCooldown(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	b, err := store.Get(id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("bead not found")}, IsError: true}
	}
	retry, _ := b.Extra["execute-loop-retry-after"].(string)
	lastStatus, _ := b.Extra["execute-loop-last-status"].(string)
	lastDetail, _ := b.Extra["execute-loop-last-detail"].(string)
	data, _ := json.Marshal(struct {
		BeadID     string `json:"bead_id"`
		RetryAfter string `json:"retry_after,omitempty"`
		LastStatus string `json:"last_status,omitempty"`
		LastDetail string `json:"last_detail,omitempty"`
	}{BeadID: b.ID, RetryAfter: retry, LastStatus: lastStatus, LastDetail: lastDetail})
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpBeadRouting(workingDir, id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("id is required")}, IsError: true}
	}
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	events, err := store.EventsByKind(id, "routing")
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("bead not found")}, IsError: true}
	}
	parsed := make([]beadRoutingEvent, 0, len(events))
	for _, e := range events {
		re := beadRoutingEvent{CreatedAt: e.CreatedAt, Summary: e.Summary}
		if e.Body != "" {
			_ = json.Unmarshal([]byte(e.Body), &re)
		}
		parsed = append(parsed, re)
	}
	data, _ := json.Marshal(parsed)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

// handleArtifactContent serves the raw file content for a DDx-tracked artifact.
// The artifact is identified by its project-relative path in the "path" query
// parameter. The response Content-Type is inferred from the file extension.
// This endpoint is used by the web UI to render binary artifact types (images,
// PDFs) that cannot be embedded in a GraphQL text response.
func (s *Server) handleArtifactContent(w http.ResponseWriter, r *http.Request) {
	entry, ok := projectFromContext(r.Context())
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		http.Error(w, "path query parameter is required", http.StatusBadRequest)
		return
	}

	// Normalize and validate path stays within project root.
	cleanedPath := filepath.Clean(filepath.FromSlash(rawPath))
	if filepath.IsAbs(cleanedPath) || cleanedPath == ".." || strings.HasPrefix(cleanedPath, ".."+string(filepath.Separator)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	absPath := filepath.Join(entry.Path, cleanedPath)
	// Double-check the resolved path is under the project root.
	rootClean := filepath.Clean(entry.Path) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(absPath)+string(filepath.Separator), rootClean) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "failed to read file", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "failed to stat file", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// handleRunBundleDownload streams a single file from a run's evidence bundle
// directory. The path query parameter is canonicalised under the bundle root
// (rejects path traversal, absolute paths, and symlink escape). All file types
// are downloadable; the response sets Content-Disposition: attachment with
// the basename so browsers prompt to save instead of rendering inline.
func (s *Server) handleRunBundleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		http.Error(w, "path query parameter is required", http.StatusBadRequest)
		return
	}
	bundleRoot, _, ok := s.state.runBundleRoot(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	full, err := resolveRunBundlePath(bundleRoot, rawPath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "failed to read file", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.Error(w, "not a regular file", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", mimeTypeForPath(rawPath))
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filepath.Base(rawPath)))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
