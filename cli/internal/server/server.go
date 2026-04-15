package server

import (
	"bufio"
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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/DocumentDrivenDX/ddx/internal/processmetrics"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
	"tailscale.com/tsnet"
)

// Server is the DDx HTTP server exposing REST and MCP endpoints.
type Server struct {
	Addr        string
	WorkingDir  string
	TsnetConfig *config.TsnetConfig
	mux         *http.ServeMux
	startTime   time.Time
	workers     *WorkerManager
	beadHub     *bead.WatcherHub
	state       *ServerState
}

// New creates a new DDx server bound to addr, serving data from workingDir.
func New(addr, workingDir string) *Server {
	nodeName := resolveNodeName()
	stateDir := serverAddrDir() // XDG-standard user-level dir, one per node
	state := loadServerState(stateDir, nodeName)
	state.workingDir = workingDir

	workers := NewWorkerManager(workingDir)
	beadHub := bead.NewWatcherHub(250 * time.Millisecond)
	s := &Server{
		Addr:       addr,
		WorkingDir: workingDir,
		mux:        http.NewServeMux(),
		startTime:  time.Now().UTC(),
		workers:    workers,
		beadHub:    beadHub,
		state:      state,
	}
	state.coordinatorReg = workers.LandCoordinators

	// Register the server's own project immediately.
	state.RegisterProject(workingDir)
	_ = state.save()

	s.routes()
	return s
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

// ListenAndServe starts the server. If TsnetConfig.Enabled is true, a parallel
// Tailscale ts-net listener is started alongside the standard localhost listener.
func (s *Server) ListenAndServe() error {
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
	if certFile == "" || keyFile == "" {
		tlsDir := filepath.Join(s.WorkingDir, ".ddx", "server", "tls")
		var err error
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
// addr file. Returns "" if none is present.
func ReadServerAddr() string {
	type addrFile struct {
		URL string `json:"url"`
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
	return af.URL
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

func (s *Server) routes() {
	// Health
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/ready", s.handleReady)

	// Node + project registry
	s.mux.HandleFunc("GET /api/node", s.handleGetNode)
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /api/projects/register", s.handleRegisterProject)
	s.mux.HandleFunc("GET /api/projects/{project}/commits", s.handleProjectCommits)

	// Documents
	s.mux.HandleFunc("GET /api/documents", s.handleListDocuments)
	s.mux.HandleFunc("PUT /api/documents/{path...}", s.handleWriteDocument)
	s.mux.HandleFunc("GET /api/documents/{path...}", s.handleReadDocument)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/personas", s.handleListPersonas)
	s.mux.HandleFunc("GET /api/personas/{role}", s.handleResolvePersona)

	// Beads
	s.mux.HandleFunc("GET /api/beads", s.handleListBeads)
	s.mux.HandleFunc("GET /api/beads/ready", s.handleBeadsReady)
	s.mux.HandleFunc("GET /api/beads/blocked", s.handleBeadsBlocked)
	s.mux.HandleFunc("GET /api/beads/status", s.handleBeadsStatus)
	s.mux.HandleFunc("GET /api/beads/dep/tree/{id}", s.handleBeadDepTree)
	s.mux.HandleFunc("GET /api/beads/{id}", s.handleShowBead)

	// Doc graph
	s.mux.HandleFunc("GET /api/docs/graph", s.handleDocGraph)
	s.mux.HandleFunc("GET /api/docs/stale", s.handleDocStale)
	s.mux.HandleFunc("GET /api/docs/{id}/deps", s.handleDocDeps)
	s.mux.HandleFunc("GET /api/docs/{id}/dependents", s.handleDocDependents)
	s.mux.HandleFunc("GET /api/docs/{id}/history", s.handleDocHistory)
	s.mux.HandleFunc("GET /api/docs/{id}/diff", s.handleDocDiff)
	s.mux.HandleFunc("PUT /api/docs/{id}", s.handleDocWrite)
	s.mux.HandleFunc("GET /api/docs/{id}", s.handleDocShow)

	// Bead mutations
	s.mux.HandleFunc("POST /api/beads", s.handleCreateBead)
	s.mux.HandleFunc("PUT /api/beads/{id}", s.handleUpdateBead)
	s.mux.HandleFunc("POST /api/beads/{id}/claim", s.handleClaimBead)
	s.mux.HandleFunc("POST /api/beads/{id}/unclaim", s.handleUnclaimBead)
	s.mux.HandleFunc("POST /api/beads/{id}/reopen", s.handleReopenBead)
	s.mux.HandleFunc("POST /api/beads/{id}/deps", s.handleBeadDeps)

	// Execution dispatch (localhost-only)
	s.mux.HandleFunc("POST /api/exec/run/{id}", s.handleExecDispatch)
	s.mux.HandleFunc("POST /api/agent/run", s.handleAgentDispatch)
	s.mux.HandleFunc("GET /api/agent/workers", s.handleAgentWorkers)
	s.mux.HandleFunc("POST /api/agent/workers/execute-loop", s.handleStartExecuteLoopWorker)
	s.mux.HandleFunc("GET /api/agent/workers/{id}", s.handleAgentWorkerShow)
	s.mux.HandleFunc("POST /api/agent/workers/{id}/stop", s.handleStopAgentWorker)
	s.mux.HandleFunc("GET /api/agent/workers/{id}/log", s.handleAgentWorkerLog)
	s.mux.HandleFunc("GET /api/agent/workers/{id}/prompt", s.handleAgentWorkerPrompt)
	s.mux.HandleFunc("GET /api/agent/coordinators", s.handleAgentCoordinators)

	// Project-scoped worker endpoints (FEAT-002 §22-24)
	s.mux.HandleFunc("GET /api/projects/{project}/workers", s.handleProjectWorkerList)
	s.mux.HandleFunc("GET /api/projects/{project}/workers/{id}/progress", s.handleProjectWorkerProgress)
	s.mux.HandleFunc("GET /api/projects/{project}/workers/{id}", s.handleProjectWorkerShow)

	// Executions
	s.mux.HandleFunc("GET /api/exec/definitions/{id}", s.handleExecDefinitionShow)
	s.mux.HandleFunc("GET /api/exec/definitions", s.handleExecDefinitions)
	s.mux.HandleFunc("GET /api/exec/runs/{id}/log", s.handleExecRunLog)
	s.mux.HandleFunc("GET /api/exec/runs/{id}", s.handleExecRunShow)
	s.mux.HandleFunc("GET /api/exec/runs", s.handleExecRuns)

	// Agent sessions
	s.mux.HandleFunc("GET /api/agent/sessions/{id}", s.handleAgentSessionDetail)
	s.mux.HandleFunc("GET /api/agent/sessions", s.handleAgentSessions)

	// Process metrics
	s.mux.HandleFunc("GET /api/metrics/summary", s.handleMetricsSummary)
	s.mux.HandleFunc("GET /api/metrics/cost", s.handleMetricsCost)
	s.mux.HandleFunc("GET /api/metrics/cycle-time", s.handleMetricsCycleTime)
	s.mux.HandleFunc("GET /api/metrics/rework", s.handleMetricsRework)

	// Providers (FEAT-002 §26-27, host+user global — not project-scoped)
	s.mux.HandleFunc("GET /api/providers", s.handleListProviders)
	s.mux.HandleFunc("GET /api/providers/{harness}", s.handleShowProvider)

	// MCP
	s.mux.HandleFunc("POST /mcp", s.handleMCP)

	// GraphQL (gqlgen) — POST for queries/mutations, GET for WebSocket subscriptions
	s.mux.HandleFunc("POST /graphql", s.handleGraphQLQuery)
	s.mux.HandleFunc("GET /graphql", s.handleGraphQLQuery)
	s.mux.HandleFunc("GET /graphiql", s.handleGraphiQL)

	// SvelteKit SPA — serve embedded frontend/build; fall back to index.html for deep links.
	sub, err := fs.Sub(frontendFiles, "frontend/build")
	if err != nil {
		panic("embed: frontend/build not found: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Attempt to serve the exact file. If not found, serve index.html so the
		// SvelteKit client-side router can handle the URL.
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
		"name":       node.Name,
		"id":         node.ID,
		"started_at": node.StartedAt,
		"last_seen":  node.LastSeen,
	})
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

	args := []string{"-C", entry.Path, "log", format, "-n", strconv.Itoa(limit)}
	if since != "" {
		args = append(args, "--since="+since)
	}
	if author != "" {
		args = append(args, "--author="+author)
	}

	cmd := exec.Command("git", args...) //nolint:gosec
	// Scrub inherited GIT_* env vars so `-C` takes effect reliably even when
	// running inside a parent git hook context.
	cmd.Env = scrubbedGitEnv()
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
	libPath := s.libraryPath()
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

	libPath := s.libraryPath()
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

	writeJSON(w, http.StatusOK, map[string]string{
		"path":    cleaned,
		"content": string(data),
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

	libPath := s.libraryPath()
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

	libPath := s.libraryPath()
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

	store := s.beadStore()
	b, err := store.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bead not found"})
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleBeadsReady(w http.ResponseWriter, r *http.Request) {
	store := s.beadStore()
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
	store := s.beadStore()
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
	store := s.beadStore()
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

	store := s.beadStore()
	tree, err := store.DepTree(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "tree": tree})
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

	store := s.beadStore()
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

	store := s.beadStore()
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

	store := s.beadStore()
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

	store := s.beadStore()
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

	store := s.beadStore()
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

	store := s.beadStore()
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
	graph, err := s.buildDocGraph()
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
	graph, err := s.buildDocGraph()
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

func (s *Server) handleDocShow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	graph, err := s.buildDocGraph()
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

	graph, err := s.buildDocGraph()
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

	graph, err := s.buildDocGraph()
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

	graph, err := s.buildDocGraph()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	doc, ok := graph.Show(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}

	// doc.Path is already an absolute path from the docgraph.
	fullPath := doc.Path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(s.WorkingDir, fullPath)
	}
	if err := os.WriteFile(fullPath, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	committed := false
	var acCfg internalgit.AutoCommitConfig
	if cfg, cfgErr := config.LoadWithWorkingDir(s.WorkingDir); cfgErr == nil && cfg.Git != nil {
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

	graph, err := s.buildDocGraph()
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
	gitCmd := exec.Command("git", gitArgs...)
	gitCmd.Dir = s.WorkingDir
	gitCmd.Env = scrubbedGitEnv()
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

	graph, err := s.buildDocGraph()
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

	diffCmd := exec.Command("git", gitArgs...)
	diffCmd.Dir = s.WorkingDir
	diffCmd.Env = scrubbedGitEnv()
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
	store := s.execStore()
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
	store := s.execStore()
	def, err := store.ShowDefinition(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (s *Server) handleExecRuns(w http.ResponseWriter, r *http.Request) {
	store := s.execStore()
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
	store := s.execStore()
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
	store := s.execStore()
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

	store := s.execStore()
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

	runner := agent.NewRunner(agent.Config{})
	opts := agent.RunOptions{
		Harness: req.Harness,
		Prompt:  req.Prompt,
		Model:   req.Model,
		Effort:  req.Effort,
		WorkDir: s.WorkingDir,
	}
	result, err := runner.Run(opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAgentWorkers(w http.ResponseWriter, r *http.Request) {
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
		Provider      string `json:"provider"`
		ModelRef      string `json:"model_ref"`
		Effort        string `json:"effort"`
		Once          bool   `json:"once"`
		PollInterval  string `json:"poll_interval"`
		NoReview      bool   `json:"no_review"`
		ReviewHarness string `json:"review_harness"`
		ReviewModel   string `json:"review_model"`
		MinTier       string `json:"min_tier"`
		MaxTier       string `json:"max_tier"`
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

	var pollInterval time.Duration
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
		Provider:      req.Provider,
		ModelRef:      req.ModelRef,
		Effort:        req.Effort,
		Once:          req.Once,
		PollInterval:  pollInterval,
		NoReview:      req.NoReview,
		ReviewHarness: req.ReviewHarness,
		ReviewModel:   req.ReviewModel,
		MinTier:       req.MinTier,
		MaxTier:       req.MaxTier,
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
	record, err := s.workers.Show(id)
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

func (s *Server) handleAgentWorkerLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	stdout, stderr, err := s.workers.Logs(id)
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
	record, err := s.workers.Show(id)
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
	promptPath := filepath.Join(s.WorkingDir, ".ddx", "executions", attemptID, "prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "prompt not found"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
}

func (s *Server) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.loadSessions()
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

	sessions, err := s.loadSessions()
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no sessions found"})
		return
	}

	for _, sess := range sessions {
		if sess.ID == id {
			writeJSON(w, http.StatusOK, detailAgentSession(sess))
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
}

func (s *Server) loadSessions() ([]agent.SessionEntry, error) {
	logFile := filepath.Join(s.WorkingDir, agent.DefaultLogDir, "sessions.jsonl")
	f, err := os.Open(logFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sessions []agent.SessionEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry agent.SessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		sessions = append(sessions, entry)
	}
	return sessions, scanner.Err()
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
		detail.Prompt = sess.Prompt
	}
	if detail.ResponseAvailable {
		detail.Response = sess.Response
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
	report, err := processmetrics.New(s.WorkingDir).Summary(query)
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
	report, err := processmetrics.New(s.WorkingDir).Cost(query)
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
	report, err := processmetrics.New(s.WorkingDir).CycleTime(query)
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
	report, err := processmetrics.New(s.WorkingDir).Rework(query)
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

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
	return []mcpTool{
		{
			Name:        "ddx_list_documents",
			Description: "List documents in the DDx library",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_read_document",
			Description: "Read the content of a document from the DDx library",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Document path relative to library root"},
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
					"query": map[string]any{"type": "string", "description": "Search query"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "ddx_resolve_persona",
			Description: "Resolve the persona bound to a role",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role": map[string]any{"type": "string", "description": "Role name to resolve"},
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
					"status": map[string]any{"type": "string", "description": "Filter by status (open, in_progress, closed)"},
					"label":  map[string]any{"type": "string", "description": "Filter by label"},
				},
			},
		},
		{
			Name:        "ddx_show_bead",
			Description: "Show details of a specific bead",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Bead ID"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "ddx_bead_ready",
			Description: "List ready beads (open with all dependencies closed)",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_bead_status",
			Description: "Get bead summary counts by status",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_doc_graph",
			Description: "Get the full document dependency graph",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_doc_stale",
			Description: "List stale documents",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "ddx_doc_show",
			Description: "Show document metadata and staleness",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Document ID"},
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
					"id": map[string]any{"type": "string", "description": "Document ID"},
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
				},
			},
		},
		{
			Name:        "ddx_exec_show",
			Description: "Show a specific execution definition",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Definition ID"},
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
				},
			},
		},
		{
			Name:        "ddx_exec_dispatch",
			Description: "Dispatch an execution run by definition ID (localhost-only)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Execution definition ID"},
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
					"since": map[string]any{"type": "string", "description": "Git ref to compare from (default: HEAD~5)"},
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
					"id": map[string]any{"type": "string", "description": "Document artifact ID"},
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
					"id":  map[string]any{"type": "string", "description": "Document artifact ID"},
					"ref": map[string]any{"type": "string", "description": "Git ref to diff against (default: working copy vs HEAD)"},
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
	}
}

func (s *Server) mcpCallTool(params json.RawMessage, r *http.Request) mcpToolResult {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "invalid tool call parameters"}},
			IsError: true,
		}
	}

	switch call.Name {
	case "ddx_list_documents":
		return s.mcpListDocuments()
	case "ddx_read_document":
		path, _ := call.Arguments["path"].(string)
		return s.mcpReadDocument(path)
	case "ddx_search":
		query, _ := call.Arguments["query"].(string)
		return s.mcpSearch(query)
	case "ddx_resolve_persona":
		role, _ := call.Arguments["role"].(string)
		return s.mcpResolvePersona(role)
	case "ddx_list_beads":
		status, _ := call.Arguments["status"].(string)
		label, _ := call.Arguments["label"].(string)
		return s.mcpListBeads(status, label)
	case "ddx_show_bead":
		id, _ := call.Arguments["id"].(string)
		return s.mcpShowBead(id)
	case "ddx_bead_ready":
		return s.mcpBeadReady()
	case "ddx_bead_status":
		return s.mcpBeadStatus()
	case "ddx_doc_graph":
		return s.mcpDocGraph()
	case "ddx_doc_stale":
		return s.mcpDocStale()
	case "ddx_doc_show":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocShow(id)
	case "ddx_doc_deps":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocDeps(id)
	case "ddx_agent_sessions":
		harness, _ := call.Arguments["harness"].(string)
		return s.mcpAgentSessions(harness)
	case "ddx_provider_list":
		return s.mcpProviderList()
	case "ddx_provider_show":
		harness, _ := call.Arguments["harness"].(string)
		return s.mcpProviderShow(harness)
	case "ddx_bead_create":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "forbidden: write tools require trusted origin"}}, IsError: true}
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
		return s.mcpBeadCreate(title, issueType, priority, labelsStr, description, acceptance)
	case "ddx_bead_update":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "forbidden: write tools require trusted origin"}}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		status, _ := call.Arguments["status"].(string)
		labelsStr, _ := call.Arguments["labels"].(string)
		description, _ := call.Arguments["description"].(string)
		acceptance, _ := call.Arguments["acceptance"].(string)
		return s.mcpBeadUpdate(id, status, labelsStr, description, acceptance)
	case "ddx_bead_claim":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "forbidden: write tools require trusted origin"}}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		assignee, _ := call.Arguments["assignee"].(string)
		return s.mcpBeadClaim(id, assignee)
	case "ddx_exec_definitions":
		artifact, _ := call.Arguments["artifact"].(string)
		return s.mcpExecDefinitions(artifact)
	case "ddx_exec_show":
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecShow(id)
	case "ddx_exec_history":
		artifact, _ := call.Arguments["artifact"].(string)
		definition, _ := call.Arguments["definition"].(string)
		return s.mcpExecHistory(artifact, definition)
	case "ddx_exec_dispatch":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "forbidden: dispatch tools require trusted origin"}}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecDispatch(id)
	case "ddx_agent_dispatch":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "forbidden: dispatch tools require trusted origin"}}, IsError: true}
		}
		harness, _ := call.Arguments["harness"].(string)
		prompt, _ := call.Arguments["prompt"].(string)
		model, _ := call.Arguments["model"].(string)
		effort, _ := call.Arguments["effort"].(string)
		return s.mcpAgentDispatch(harness, prompt, model, effort)
	case "ddx_doc_changed":
		since, _ := call.Arguments["since"].(string)
		return s.mcpDocChanged(since)
	case "ddx_doc_write":
		if !isTrusted(r) {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "forbidden: write tools require trusted origin"}}, IsError: true}
		}
		id, _ := call.Arguments["id"].(string)
		content, _ := call.Arguments["content"].(string)
		return s.mcpDocWrite(id, content)
	case "ddx_doc_history":
		id, _ := call.Arguments["id"].(string)
		return s.mcpDocHistory(id)
	case "ddx_doc_diff":
		id, _ := call.Arguments["id"].(string)
		ref, _ := call.Arguments["ref"].(string)
		return s.mcpDocDiff(id, ref)
	case "ddx_list_projects":
		return s.mcpListProjects()
	case "ddx_show_project":
		id, _ := call.Arguments["id"].(string)
		path, _ := call.Arguments["path"].(string)
		return s.mcpShowProject(id, path)
	default:
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", call.Name)}},
			IsError: true,
		}
	}
}

// --- MCP Tool Implementations ---

func (s *Server) mcpListDocuments() mcpToolResult {
	libPath := s.libraryPath()
	if libPath == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpReadDocument(path string) mcpToolResult {
	if path == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "path is required"}},
			IsError: true,
		}
	}
	libPath := s.libraryPath()
	if libPath == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "library not configured"}},
			IsError: true,
		}
	}
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "invalid path"}},
			IsError: true,
		}
	}
	data, err := os.ReadFile(filepath.Join(libPath, cleaned))
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "document not found"}},
			IsError: true,
		}
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpSearch(query string) mcpToolResult {
	if query == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "query is required"}},
			IsError: true,
		}
	}

	libPath := s.libraryPath()
	if libPath == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpResolvePersona(role string) mcpToolResult {
	if role == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "role is required"}},
			IsError: true,
		}
	}

	bm := persona.NewBindingManagerWithPath(filepath.Join(s.WorkingDir, ".ddx.yml"))
	personaName, err := bm.GetBinding(role)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("no persona bound to role: %s", role)}},
			IsError: true,
		}
	}

	loader := persona.NewPersonaLoader(s.WorkingDir)
	p, err := loader.LoadPersona(personaName)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("persona not found: %s", personaName)}},
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpListBeads(status, label string) mcpToolResult {
	store := s.beadStore()
	beads, err := store.List(status, label, nil)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
	}
	if beads == nil {
		beads = []bead.Bead{}
	}
	data, _ := json.Marshal(beads)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpShowBead(id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id is required"}},
			IsError: true,
		}
	}
	store := s.beadStore()
	b, err := store.Get(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "bead not found"}},
			IsError: true,
		}
	}
	data, _ := json.Marshal(b)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpListProjects() mcpToolResult {
	projects := s.state.GetProjects()
	if projects == nil {
		projects = []ProjectEntry{}
	}
	data, _ := json.Marshal(projects)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpShowProject(id, path string) mcpToolResult {
	if id == "" && path == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id or path is required"}},
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
			Content: []mcpContent{{Type: "text", Text: "project not found"}},
			IsError: true,
		}
	}
	data, _ := json.Marshal(entry)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpBeadReady() mcpToolResult {
	store := s.beadStore()
	ready, err := store.Ready()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
	}
	if ready == nil {
		ready = []bead.Bead{}
	}
	data, _ := json.Marshal(ready)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpBeadStatus() mcpToolResult {
	store := s.beadStore()
	counts, err := store.Status()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf(`{"error":"%s"}`, err.Error())}},
			IsError: true,
		}
	}
	data, _ := json.Marshal(counts)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocGraph() mcpToolResult {
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf(`{"error":"%s"}`, err.Error())}},
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocStale() mcpToolResult {
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf(`{"error":"%s"}`, err.Error())}},
			IsError: true,
		}
	}
	stale := graph.StaleDocs()
	if stale == nil {
		stale = []docgraph.StaleReason{}
	}
	data, _ := json.Marshal(stale)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocShow(id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id is required"}},
			IsError: true,
		}
	}
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf(`{"error":"%s"}`, err.Error())}},
			IsError: true,
		}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "document not found"}},
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocDeps(id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id is required"}},
			IsError: true,
		}
	}
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf(`{"error":"%s"}`, err.Error())}},
			IsError: true,
		}
	}
	deps, err := graph.Dependencies(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	data, _ := json.Marshal(deps)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpAgentSessions(harness string) mcpToolResult {
	sessions, err := s.loadSessions()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpBeadCreate(title, issueType string, priority int, labelsStr, description, acceptance string) mcpToolResult {
	if title == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "title is required"}},
			IsError: true,
		}
	}
	store := s.beadStore()
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
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	data, _ := json.Marshal(b)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpBeadUpdate(id, status, labelsStr, description, acceptance string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id is required"}},
			IsError: true,
		}
	}
	store := s.beadStore()
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
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	updated, _ := store.Get(id)
	data, _ := json.Marshal(updated)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpBeadClaim(id, assignee string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id is required"}},
			IsError: true,
		}
	}
	store := s.beadStore()
	if err := store.ClaimWithOptions(id, assignee, "", ""); err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: fmt.Sprintf(`{"id":"%s","status":"claimed"}`, id)}}}
}

func (s *Server) mcpExecDefinitions(artifactID string) mcpToolResult {
	store := s.execStore()
	defs, err := store.ListDefinitions(artifactID)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
	}
	data, _ := json.Marshal(defs)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpExecShow(id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "id is required"}},
			IsError: true,
		}
	}
	store := s.execStore()
	def, err := store.ShowDefinition(id)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	data, _ := json.Marshal(def)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpExecHistory(artifactID, definitionID string) mcpToolResult {
	store := s.execStore()
	runs, err := store.History(artifactID, definitionID)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
	}
	data, _ := json.Marshal(runs)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocWrite(id, content string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "id is required"}}, IsError: true}
	}
	if content == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "content is required"}}, IsError: true}
	}
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "document not found"}}, IsError: true}
	}
	// doc.Path is already an absolute path from the docgraph.
	fullPath := doc.Path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(s.WorkingDir, fullPath)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	committed := false
	var acCfg internalgit.AutoCommitConfig
	if cfg, cfgErr := config.LoadWithWorkingDir(s.WorkingDir); cfgErr == nil && cfg.Git != nil {
		acCfg.AutoCommit = cfg.Git.AutoCommit
		acCfg.CommitPrefix = cfg.Git.CommitPrefix
	}
	if acCfg.AutoCommit == "always" {
		if _, acErr := internalgit.AutoCommit(fullPath, id, "write document", acCfg); acErr == nil {
			committed = true
		}
	}
	data, _ := json.Marshal(map[string]any{"status": "ok", "path": doc.Path, "committed": committed})
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocHistory(id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "id is required"}}, IsError: true}
	}
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "document not found"}}, IsError: true}
	}
	logCmd := exec.Command("git", "log", "--follow", "--format=%H\t%ai\t%an\t%s", "--", doc.Path)
	logCmd.Dir = s.WorkingDir
	out, gitErr := logCmd.Output()
	if gitErr != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "git log failed"}}, IsError: true}
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
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocDiff(id, ref string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "id is required"}}, IsError: true}
	}
	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	doc, ok := graph.Show(id)
	if !ok {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "document not found"}}, IsError: true}
	}
	var gitArgs []string
	if ref != "" {
		gitArgs = []string{"diff", ref, "--", doc.Path}
	} else {
		gitArgs = []string{"diff", "--", doc.Path}
	}
	mcpDiffCmd := exec.Command("git", gitArgs...)
	mcpDiffCmd.Dir = s.WorkingDir
	out, gitErr := mcpDiffCmd.Output()
	if gitErr != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "git diff failed"}}, IsError: true}
	}
	data, _ := json.Marshal(map[string]string{"diff": string(out)})
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpExecDispatch(id string) mcpToolResult {
	if id == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "id is required"}}, IsError: true}
	}
	store := s.execStore()
	record, err := store.Run(context.Background(), id)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	data, _ := json.Marshal(record)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpAgentDispatch(harness, prompt, model, effort string) mcpToolResult {
	if harness == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "harness is required"}}, IsError: true}
	}
	if prompt == "" {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "prompt is required"}}, IsError: true}
	}
	runner := agent.NewRunner(agent.Config{})
	opts := agent.RunOptions{
		Harness: harness,
		Prompt:  prompt,
		Model:   model,
		Effort:  effort,
		WorkDir: s.WorkingDir,
	}
	result, err := runner.Run(opts)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	data, _ := json.Marshal(result)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

func (s *Server) mcpDocChanged(since string) mcpToolResult {
	if since == "" {
		since = "HEAD~5"
	}
	diffCmd := exec.Command("git", "diff", "--name-status", since, "HEAD")
	diffCmd.Dir = s.WorkingDir
	out, gitErr := diffCmd.Output()
	if gitErr != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "git diff failed"}}, IsError: true}
	}

	graph, err := s.buildDocGraph()
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}

	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootCmd.Dir = s.WorkingDir
	rootOut, rootErr := rootCmd.Output()
	if rootErr != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "could not determine git root"}}, IsError: true}
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

		var changeType string
		switch {
		case strings.HasPrefix(statusCode, "A"):
			changeType = "added"
		case strings.HasPrefix(statusCode, "D"):
			changeType = "deleted"
		default:
			changeType = "modified"
		}

		if id, ok := graph.PathToID[cleanPath]; ok {
			entries = append(entries, changedEntry{ID: id, Path: relPath, ChangeType: changeType})
		}
	}

	data, _ := json.Marshal(entries)
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

// --- Helpers ---

// scrubbedGitEnv returns the current process environment with GIT_* variables
// removed. This ensures git subcommands invoked by the server honour explicit
// -C <dir>/cmd.Dir settings instead of inheriting e.g. GIT_DIR / GIT_WORK_TREE
// from a parent git hook.
func scrubbedGitEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func (s *Server) libraryPath() string {
	cfg, err := config.LoadWithWorkingDir(s.WorkingDir)
	if err != nil {
		return ""
	}
	if cfg.Library == nil || cfg.Library.Path == "" {
		return ""
	}
	p := cfg.Library.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.WorkingDir, p)
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
	// Create gqlgen server with the DDX GraphQL schema
	gqlServer := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:        s.state,
			WorkingDir:   s.WorkingDir,
			Workers:      s.workers,
			BeadBus:      s.beadHub,
			ExecLogs:     &execLogAdapter{workingDir: s.WorkingDir},
			CoordMetrics: &coordMetricsAdapter{reg: s.workers.LandCoordinators},
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

	gqlServer.ServeHTTP(w, r)
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
