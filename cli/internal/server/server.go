package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/easel/ddx/internal/bead"
	"github.com/easel/ddx/internal/config"
	"github.com/easel/ddx/internal/docgraph"
)

// Server is the DDx HTTP server exposing REST and MCP endpoints.
type Server struct {
	Addr       string
	WorkingDir string
	mux        *http.ServeMux
}

// New creates a new DDx server bound to addr, serving data from workingDir.
func New(addr, workingDir string) *Server {
	s := &Server{
		Addr:       addr,
		WorkingDir: workingDir,
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the server's HTTP handler (useful for testing).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/documents", s.handleListDocuments)
	s.mux.HandleFunc("GET /api/documents/{path...}", s.handleReadDocument)
	s.mux.HandleFunc("GET /api/beads", s.handleListBeads)
	s.mux.HandleFunc("GET /api/beads/ready", s.handleBeadsReady)
	s.mux.HandleFunc("GET /api/beads/status", s.handleBeadsStatus)
	s.mux.HandleFunc("GET /api/docs/graph", s.handleDocGraph)
	s.mux.HandleFunc("GET /api/docs/stale", s.handleDocStale)
	s.mux.HandleFunc("POST /mcp", s.handleMCP)
}

// --- HTTP API Handlers ---

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
		http.Error(w, `{"error":"path required"}`, http.StatusBadRequest)
		return
	}

	libPath := s.libraryPath()
	if libPath == "" {
		http.Error(w, `{"error":"library not configured"}`, http.StatusNotFound)
		return
	}

	// Prevent path traversal
	cleaned := filepath.Clean(docPath)
	if strings.Contains(cleaned, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(libPath, cleaned)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, `{"error":"document not found"}`, http.StatusNotFound)
		return
	}

	type docContent struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	writeJSON(w, http.StatusOK, docContent{Path: cleaned, Content: string(data)})
}

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

func (s *Server) handleBeadsStatus(w http.ResponseWriter, r *http.Request) {
	store := s.beadStore()
	counts, err := store.Status()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, counts)
}

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
	writeJSON(w, http.StatusOK, stale)
}

func (s *Server) buildDocGraph() (*docgraph.Graph, error) {
	return docgraph.BuildGraphWithConfig(s.WorkingDir)
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
		// Client acknowledgement, no response needed per spec.
		// But since we received a JSON-RPC request, return empty result.
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
					"path": map[string]any{
						"type":        "string",
						"description": "Document path relative to library root",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "ddx_list_beads",
			Description: "List work items (beads) with optional filters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type":        "string",
						"description": "Filter by status (open, in_progress, closed)",
					},
					"label": map[string]any{
						"type":        "string",
						"description": "Filter by label",
					},
				},
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
	case "ddx_list_beads":
		status, _ := call.Arguments["status"].(string)
		label, _ := call.Arguments["label"].(string)
		return s.mcpListBeads(status, label)
	case "ddx_bead_ready":
		return s.mcpBeadReady()
	default:
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", call.Name)}},
			IsError: true,
		}
	}
}

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

func (s *Server) mcpListBeads(status, label string) mcpToolResult {
	store := s.beadStore()
	beads, err := store.List(status, label)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "[]"}}}
	}
	if beads == nil {
		beads = []bead.Bead{}
	}
	data, _ := json.Marshal(beads)
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
