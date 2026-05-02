package server

import (
	"errors"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// runBundleInlineMaxBytes is the per-file cap for inline content returned by
// runBundleFile. Files larger than this return truncated=true with no body so
// the client can fall back to a download path.
const runBundleInlineMaxBytes = 64 * 1024

// runBundleInlineExtensions is the set of file extensions and exact filenames
// whose contents may be inlined (when also under the size cap). Anything
// outside this set is download-only.
var runBundleInlineExtensions = map[string]bool{
	".txt":          true,
	".md":           true,
	"manifest.json": true,
	"prompt.md":     true,
	"result.json":   true,
}

// errRunBundleOutsideRoot is returned by resolveRunBundlePath when the
// requested path canonicalises outside the run's bundle root.
var errRunBundleOutsideRoot = errors.New("run bundle: path outside bundle root")

// runBundleRoot returns the canonical absolute filesystem path for the run's
// bundle directory, plus the project the run belongs to. The second return
// value is the project root; the third is true when a usable bundle exists.
func (s *ServerState) runBundleRoot(runID string) (bundleRoot, projectRoot string, ok bool) {
	if runID == "" {
		return "", "", false
	}
	for _, proj := range s.GetProjects(false) {
		bundleID := bundleIDForRunID(proj.Path, runID)
		if bundleID == "" {
			continue
		}
		root := filepath.Join(proj.Path, agent.ExecuteBeadArtifactDir, bundleID)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		// Canonicalize root so symlink-escape checks compare apples to apples.
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			canonical = root
		}
		return canonical, proj.Path, true
	}
	return "", "", false
}

// bundleIDForRunID maps a Run.id to its bundle directory name under
// .ddx/executions/, or "" when the run has no bundle.
//
// Three input shapes are recognized:
//   - "exec-<bundleID>"             → try-layer synthesised from bundle directory
//   - run-layer session ID with a BundlePath pointing under .ddx/executions/
//   - bundle ID directly (for caller convenience)
func bundleIDForRunID(projectRoot, runID string) string {
	if strings.HasPrefix(runID, "exec-") {
		return strings.TrimPrefix(runID, "exec-")
	}
	// Try resolving the run-layer session entry.
	logDir := agent.SessionLogDirForWorkDir(projectRoot)
	entry, ok, err := agent.FindSessionIndex(logDir, runID)
	if err == nil && ok && entry.BundlePath != "" {
		return bundleIDFromPath(entry.BundlePath)
	}
	return ""
}

// bundleIDFromPath extracts the bundle directory leaf name from a BundlePath
// of the form ".ddx/executions/<bundleID>" or ".ddx/executions/<bundleID>/...".
func bundleIDFromPath(p string) string {
	clean := filepath.ToSlash(p)
	prefix := filepath.ToSlash(agent.ExecuteBeadArtifactDir) + "/"
	idx := strings.Index(clean, prefix)
	if idx < 0 {
		return ""
	}
	rest := clean[idx+len(prefix):]
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	if !looksLikeBundleID(rest) {
		return ""
	}
	return rest
}

// resolveRunBundlePath canonicalises a user-supplied path (relative to the
// bundle root) and returns the absolute filesystem path inside the bundle.
// It rejects:
//   - absolute paths
//   - paths containing ".." that escape the root
//   - symlink targets that resolve outside the root
//
// All three rejection modes return errRunBundleOutsideRoot. The path argument
// must use forward slashes (or be normalisable to them).
func resolveRunBundlePath(bundleRoot, path string) (string, error) {
	if path == "" {
		return "", errRunBundleOutsideRoot
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return "", errRunBundleOutsideRoot
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errRunBundleOutsideRoot
	}
	full := filepath.Join(bundleRoot, clean)
	// Re-clean and confirm it sits under bundleRoot.
	full = filepath.Clean(full)
	rel, err := filepath.Rel(bundleRoot, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errRunBundleOutsideRoot
	}
	// If the file (or any parent) is a symlink, confirm the eval'd target
	// also remains inside the canonical bundleRoot. Missing files are OK to
	// pass through here — the caller will return 404 when reading.
	if eval, err := filepath.EvalSymlinks(full); err == nil {
		evalClean := filepath.Clean(eval)
		rel2, err := filepath.Rel(bundleRoot, evalClean)
		if err != nil || rel2 == ".." || strings.HasPrefix(rel2, ".."+string(filepath.Separator)) {
			return "", errRunBundleOutsideRoot
		}
	}
	return full, nil
}

// listRunBundleFiles walks the bundle root and returns one RunBundleFile per
// regular file. Symlinks pointing outside the root are skipped.
func listRunBundleFiles(bundleRoot string) []*ddxgraphql.RunBundleFile {
	var out []*ddxgraphql.RunBundleFile
	if bundleRoot == "" {
		return out
	}
	_ = filepath.Walk(bundleRoot, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		// Skip anything that, after symlink resolution, escapes the root.
		if eval, err := filepath.EvalSymlinks(p); err == nil {
			rel, err := filepath.Rel(bundleRoot, filepath.Clean(eval))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil
			}
		}
		rel, err := filepath.Rel(bundleRoot, p)
		if err != nil {
			return nil
		}
		size := int(info.Size())
		mt := mimeTypeForPath(rel)
		out = append(out, &ddxgraphql.RunBundleFile{
			Path:     filepath.ToSlash(rel),
			Size:     size,
			MimeType: mt,
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// readRunBundleFile resolves path under bundleRoot and returns the file's
// inline content (when allowed) plus size/truncated metadata. The bool return
// is false when the file does not exist (404) or fails the confinement check.
func readRunBundleFile(bundleRoot, path string) (*ddxgraphql.RunBundleFileContent, bool) {
	full, err := resolveRunBundlePath(bundleRoot, path)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return nil, false
	}
	size := int(info.Size())
	mt := mimeTypeForPath(path)
	out := &ddxgraphql.RunBundleFileContent{
		Path:      filepath.ToSlash(path),
		SizeBytes: size,
		Truncated: false,
		MimeType:  mt,
	}
	if !runBundleInlineAllowed(path) || size > runBundleInlineMaxBytes {
		out.Truncated = true
		return out, true
	}
	data, err := os.ReadFile(full)
	if err != nil {
		out.Truncated = true
		return out, true
	}
	body := string(data)
	out.Content = &body
	return out, true
}

// runBundleInlineAllowed reports whether a path qualifies for inline content
// based on extension and exact-filename whitelist rules from Story 16.
func runBundleInlineAllowed(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	if runBundleInlineExtensions[base] {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	return runBundleInlineExtensions[ext]
}

// mimeTypeForPath returns a best-effort MIME type from the file extension.
// Falls back to application/octet-stream when nothing matches.
func mimeTypeForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".jsonl":
		return "application/x-ndjson"
	case ".log":
		return "text/plain"
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

// loadRunDetail enriches a Run model with the body fields that are only
// loaded for run(id:) lookups: prompt, response, stderr, bundleFiles. It is a
// no-op when the run has no associated bundle / session bodies.
func (s *ServerState) loadRunDetail(run *ddxgraphql.Run) {
	if run == nil {
		return
	}
	bundleRoot, projectRoot, ok := s.runBundleRoot(run.ID)
	if ok {
		run.BundleFiles = listRunBundleFiles(bundleRoot)
	} else {
		run.BundleFiles = []*ddxgraphql.RunBundleFile{}
	}
	// Populate prompt/response/stderr from the session bodies, when we can
	// resolve a session entry for this run.
	if projectRoot == "" {
		// Fall back to scanning all projects for an entry by run id.
		for _, proj := range s.GetProjects(false) {
			if entry, ok, err := agent.FindSessionIndex(agent.SessionLogDirForWorkDir(proj.Path), run.ID); err == nil && ok {
				bodies := agent.LoadSessionBodies(proj.Path, entry)
				assignBodies(run, bodies)
				return
			}
		}
		return
	}
	logDir := agent.SessionLogDirForWorkDir(projectRoot)
	if entry, ok, err := agent.FindSessionIndex(logDir, run.ID); err == nil && ok {
		bodies := agent.LoadSessionBodies(projectRoot, entry)
		assignBodies(run, bodies)
		return
	}
	// Try-layer synthesised IDs ("exec-<bundleID>") have no session entry but
	// can pull prompt/result.json directly from the bundle.
	if strings.HasPrefix(run.ID, "exec-") {
		if data, err := os.ReadFile(filepath.Join(bundleRoot, "prompt.md")); err == nil {
			s := string(data)
			run.Prompt = &s
		}
		if data, err := os.ReadFile(filepath.Join(bundleRoot, "result.json")); err == nil {
			s := string(data)
			run.Response = &s
		}
	}
}

func assignBodies(run *ddxgraphql.Run, bodies agent.SessionBodies) {
	if bodies.Prompt != "" {
		s := bodies.Prompt
		run.Prompt = &s
	}
	if bodies.Response != "" {
		s := bodies.Response
		run.Response = &s
	}
	if bodies.Stderr != "" {
		s := bodies.Stderr
		run.Stderr = &s
	}
}

// GetRunToolCallsGraphQL returns the normalized ToolCallEntry slice persisted
// at drain time for a run, projected onto the GraphQL RunToolCall type.
//
// For run-layer Runs, the source is the per-session ToolCalls field on
// SessionIndexEntry. For try-layer Runs ("exec-<bundleID>"), we union the
// child-session tool calls (multiple agent invocations may share a try).
func (s *ServerState) GetRunToolCallsGraphQL(id string) []*ddxgraphql.RunToolCall {
	if id == "" {
		return nil
	}
	for _, proj := range s.GetProjects(false) {
		logDir := agent.SessionLogDirForWorkDir(proj.Path)
		// Run-layer: direct lookup by session id.
		if entry, ok, _ := agent.FindSessionIndex(logDir, id); ok {
			return runToolCallsFromEntry(entry.ToolCalls)
		}
		// Try-layer: union all sessions that point at the same bundle.
		if strings.HasPrefix(id, "exec-") {
			bundleID := strings.TrimPrefix(id, "exec-")
			entries, err := agent.ReadSessionIndex(logDir, agent.SessionIndexQuery{})
			if err != nil {
				continue
			}
			var all []agent.ToolCallEntry
			for _, e := range entries {
				if bundleIDFromPath(e.BundlePath) != bundleID {
					continue
				}
				all = append(all, e.ToolCalls...)
			}
			if len(all) > 0 {
				return runToolCallsFromEntry(all)
			}
		}
	}
	return nil
}

func runToolCallsFromEntry(entries []agent.ToolCallEntry) []*ddxgraphql.RunToolCall {
	out := make([]*ddxgraphql.RunToolCall, 0, len(entries))
	for i, e := range entries {
		call := &ddxgraphql.RunToolCall{
			ID:    formatRunToolCallID(i),
			Seq:   i,
			Tool:  e.Tool,
			Input: e.Input,
		}
		if e.Output != "" {
			s := e.Output
			call.Output = &s
		}
		if e.Error != "" {
			s := e.Error
			call.Error = &s
		}
		if e.Duration > 0 {
			d := e.Duration
			call.DurationMs = &d
		}
		out = append(out, call)
	}
	return out
}

func formatRunToolCallID(seq int) string {
	return "rtc-" + strconv.Itoa(seq)
}

// GetRunBundleFileGraphQL is the data-layer entry point for the
// runBundleFile(id, path) resolver. Returns (nil, false) when the run is
// unknown, the path fails confinement, or the file does not exist.
func (s *ServerState) GetRunBundleFileGraphQL(id, path string) (*ddxgraphql.RunBundleFileContent, bool) {
	bundleRoot, _, ok := s.runBundleRoot(id)
	if !ok {
		return nil, false
	}
	return readRunBundleFile(bundleRoot, path)
}
