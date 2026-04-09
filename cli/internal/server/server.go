package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"tailscale.com/tsnet"
)

// Server is the DDx HTTP server exposing REST and MCP endpoints.
type Server struct {
	Addr        string
	WorkingDir  string
	TsnetConfig *config.TsnetConfig
	mux         *http.ServeMux
	startTime   time.Time
}

// New creates a new DDx server bound to addr, serving data from workingDir.
func New(addr, workingDir string) *Server {
	s := &Server{
		Addr:       addr,
		WorkingDir: workingDir,
		mux:        http.NewServeMux(),
		startTime:  time.Now().UTC(),
	}
	s.routes()
	return s
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
			errCh <- s.listenTsnet(errCh)
		}()

		return <-errCh
	}
	return http.ListenAndServe(s.Addr, s.mux)
}

// listenTsnet starts a Tailscale ts-net TLS listener and serves the same mux.
func (s *Server) listenTsnet(errCh chan error) error {
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

	// Executions
	s.mux.HandleFunc("GET /api/exec/definitions/{id}", s.handleExecDefinitionShow)
	s.mux.HandleFunc("GET /api/exec/definitions", s.handleExecDefinitions)
	s.mux.HandleFunc("GET /api/exec/runs/{id}/log", s.handleExecRunLog)
	s.mux.HandleFunc("GET /api/exec/runs/{id}", s.handleExecRunShow)
	s.mux.HandleFunc("GET /api/exec/runs", s.handleExecRuns)

	// Agent sessions
	s.mux.HandleFunc("GET /api/agent/sessions/{id}", s.handleAgentSessionDetail)
	s.mux.HandleFunc("GET /api/agent/sessions", s.handleAgentSessions)

	// MCP
	s.mux.HandleFunc("POST /mcp", s.handleMCP)

	// Web UI (embedded SPA)
	distFS, err := fs.Sub(frontendFiles, "frontend/dist")
	if err == nil {
		s.mux.Handle("/", spaHandler(http.FS(distFS)))
	}
}

// spaHandler serves static files from the embedded FS, falling back to
// index.html for client-side routes (SPA routing).
func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Try to open the file from the embedded FS
		f, err := fsys.Open(path)
		if err != nil {
			// File not found — serve index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	})
}

// --- Health Endpoints ---

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

func (s *Server) handleListBeads(w http.ResponseWriter, r *http.Request) {
	store := s.beadStore()
	beads, err := store.ReadAll()
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	status := r.URL.Query().Get("status")
	label := r.URL.Query().Get("label")
	if status != "" || label != "" {
		var filtered []bead.Bead
		for _, b := range beads {
			if status != "" && b.Status != status {
				continue
			}
			if label != "" && !containsString(b.Labels, label) {
				continue
			}
			filtered = append(filtered, b)
		}
		beads = filtered
	}

	if beads == nil {
		beads = []bead.Bead{}
	}
	writeJSON(w, http.StatusOK, beads)
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
		if acErr := internalgit.AutoCommit(fullPath, id, "write document", acCfg); acErr == nil {
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

// --- Agent Session Endpoints ---

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

	writeJSON(w, http.StatusOK, sessions)
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
			writeJSON(w, http.StatusOK, sess)
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
		resp.Result = s.mcpCallTool(req.Params)
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
	}
}

func (s *Server) mcpCallTool(params json.RawMessage) mcpToolResult {
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
	case "ddx_bead_create":
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
		id, _ := call.Arguments["id"].(string)
		status, _ := call.Arguments["status"].(string)
		labelsStr, _ := call.Arguments["labels"].(string)
		description, _ := call.Arguments["description"].(string)
		acceptance, _ := call.Arguments["acceptance"].(string)
		return s.mcpBeadUpdate(id, status, labelsStr, description, acceptance)
	case "ddx_bead_claim":
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
		id, _ := call.Arguments["id"].(string)
		return s.mcpExecDispatch(id)
	case "ddx_agent_dispatch":
		harness, _ := call.Arguments["harness"].(string)
		prompt, _ := call.Arguments["prompt"].(string)
		model, _ := call.Arguments["model"].(string)
		effort, _ := call.Arguments["effort"].(string)
		return s.mcpAgentDispatch(harness, prompt, model, effort)
	case "ddx_doc_changed":
		since, _ := call.Arguments["since"].(string)
		return s.mcpDocChanged(since)
	case "ddx_doc_write":
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
		if acErr := internalgit.AutoCommit(fullPath, id, "write document", acCfg); acErr == nil {
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
