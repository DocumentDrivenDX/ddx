package docgraph

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	frontmatterSeparator = "---"
)

var (
	ErrNotDocGraphDocument = errors.New("not a doc graph document")
)

type ReviewMetadata struct {
	SelfHash   string            `yaml:"self_hash" json:"self_hash"`
	Deps       map[string]string `yaml:"deps" json:"deps"`
	ReviewedAt string            `yaml:"reviewed_at" json:"reviewed_at"`
}

type Document struct {
	ID         string
	Path       string
	Title      string
	DependsOn  []string
	Inputs     []string
	Review     ReviewMetadata
	ParkingLot bool
	Prompt     string
	Dependents []string
	ExecDef    *DocExecDef

	contentHash string

	frontmatter   *yaml.Node
	body          string
	bodyLinkTexts []string
}

type StaleReason struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Reasons []string `json:"reasons"`
}

type Graph struct {
	RootDir    string
	Documents  map[string]*Document
	PathToID   map[string]string
	Dependents map[string][]string
	Warnings   []string
}

type GraphConfig struct {
	Roots    []string            `yaml:"roots"`
	IDToPath map[string]string   `yaml:"id_to_path"`
	Cascade  map[string][]string `yaml:"cascade"`
	IDMap    map[string]string   `yaml:"id_map"`
	Required []string            `yaml:"required_roots"`
}

func BuildGraph(workingDir string) (*Graph, error) {
	return buildGraph(workingDir, nil)
}

func BuildGraphWithConfig(workingDir string) (*Graph, error) {
	configs, err := LoadGraphConfigs(workingDir)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return BuildGraph(workingDir)
	}

	roots := make([]string, 0, len(configs))
	for _, cfg := range configs {
		for _, root := range cfg.Roots {
			if root == "" {
				continue
			}
			roots = append(roots, filepath.Join(workingDir, root))
		}
	}
	if len(roots) == 0 {
		return BuildGraph(workingDir)
	}

	graph, err := buildGraph(workingDir, roots)
	if err != nil {
		return nil, err
	}
	graph.applyConfig(configs)
	return graph, nil
}

func buildGraph(workingDir string, roots []string) (*Graph, error) {
	files, err := findMarkdownFiles(workingDir, roots)
	if err != nil {
		return nil, err
	}

	cleanRoot := filepath.Clean(workingDir)
	documents := make(map[string]*Document)
	pathToID := make(map[string]string)
	warnings := []string{}
	for _, filePath := range files {
		doc, err := ParseDocument(filePath)
		if err != nil {
			if errors.Is(err, ErrNotDocGraphDocument) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("%s: %v", filePath, err))
			continue
		}
		if _, exists := documents[doc.ID]; exists {
			warnings = append(warnings, fmt.Sprintf("duplicate document id %q in %q", doc.ID, filePath))
			continue
		}
		doc.Path = relPath(cleanRoot, filePath)
		documents[doc.ID] = doc
		pathToID[doc.Path] = doc.ID
	}

	g := &Graph{
		RootDir:   cleanRoot,
		Documents: documents,
		PathToID:  pathToID,
		Warnings:  warnings,
	}
	g.resolveBodyLinks()
	g.buildDependents()
	g.detectCycles()
	g.applyConfigDefaults()
	return g, nil
}

func (g *Graph) applyConfigDefaults() {
	for _, doc := range g.Documents {
		doc.DependsOn = dedupeSortedStrings(doc.DependsOn)
		doc.Inputs = dedupeSortedStrings(doc.Inputs)
	}
}

func (g *Graph) applyConfig(configs []GraphConfig) {
	if len(configs) == 0 {
		return
	}

	idToPath := make(map[string]string)
	cascade := make(map[string][]string)
	for _, cfg := range configs {
		for id, rawPath := range cfg.IDToPath {
			if id == "" || rawPath == "" {
				continue
			}
			idToPath[id] = filepath.Clean(filepath.Join(g.RootDir, rawPath))
		}
		for from, tos := range cfg.Cascade {
			for _, to := range tos {
				if from == "" || to == "" {
					continue
				}
				cascade[from] = append(cascade[from], to)
			}
		}
		if len(cfg.IDMap) > 0 {
			for id, rawPath := range cfg.IDMap {
				if id == "" || rawPath == "" {
					continue
				}
				idToPath[id] = filepath.Clean(filepath.Join(g.RootDir, rawPath))
			}
		}
		if len(cfg.Required) > 0 {
			for _, id := range cfg.Required {
				if _, ok := g.Documents[id]; !ok {
					g.Warnings = append(g.Warnings, fmt.Sprintf("required root document %q not found", id))
				}
			}
		}
	}

	if len(idToPath) > 0 {
		for id, path := range idToPath {
			if _, ok := g.Documents[id]; ok {
				continue
			}
			if _, err := os.Stat(path); err != nil {
				g.Warnings = append(g.Warnings, fmt.Sprintf("id_to_path entry %q -> %q does not exist", id, path))
				continue
			}
			doc, err := ParseDocument(path)
			if err != nil {
				if errors.Is(err, ErrNotDocGraphDocument) {
					g.Warnings = append(g.Warnings, fmt.Sprintf("id_to_path entry %q -> %q is not a doc graph document", id, path))
					continue
				}
				g.Warnings = append(g.Warnings, fmt.Sprintf("id_to_path entry %q -> %q could not be loaded: %v", id, path, err))
				continue
			}
			if doc.ID != id {
				g.Warnings = append(g.Warnings, fmt.Sprintf("id_to_path mismatch for %q: %q declares %q", id, path, doc.ID))
				continue
			}
			g.Documents[id] = doc
			doc.Path = relPath(g.RootDir, path)
			g.PathToID[doc.Path] = id
		}
	}

	for from, toIDs := range cascade {
		if _, ok := g.Documents[from]; !ok {
			g.Warnings = append(g.Warnings, fmt.Sprintf("cascade rule refers to unknown dependency root %q", from))
			continue
		}
		for _, to := range dedupeSortedStrings(toIDs) {
			if to == "" {
				continue
			}
			child, ok := g.Documents[to]
			if !ok {
				g.Warnings = append(g.Warnings, fmt.Sprintf("cascade rule refers to unknown target doc %q", to))
				continue
			}
			if !containsString(child.DependsOn, from) {
				child.DependsOn = append(child.DependsOn, from)
			}
		}
	}

	for id := range g.Documents {
		g.Documents[id].DependsOn = dedupeSortedStrings(g.Documents[id].DependsOn)
	}
	g.buildDependents()
	g.detectCycles()
}

func findMarkdownFiles(root string, roots []string) ([]string, error) {
	targetDirs := []string{root}
	if len(roots) > 0 {
		targetDirs = roots
	}

	files := []string{}
	for _, base := range targetDirs {
		cleanRoot := filepath.Clean(base)
		err := filepath.Walk(cleanRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info == nil {
				return nil
			}
			if info.IsDir() {
				name := filepath.Base(path)
				// Skip tool-managed directories. .claude is the Claude Code
				// workspace (including throwaway copies under .claude/worktrees/);
				// a worktrees/ directory at any depth is agent-scratch and must
				// not be surfaced as canonical documents.
				switch name {
				case ".git", ".ddx", ".claude", "worktrees":
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(filepath.Base(path), ".") {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".md" {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// relPath returns filePath expressed relative to root. If filePath cannot be
// made relative (e.g. different volumes on Windows) the cleaned absolute path
// is returned. Callers treat the result as the canonical document path.
func relPath(root, filePath string) string {
	clean := filepath.Clean(filePath)
	if rel, err := filepath.Rel(root, clean); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return rel
	}
	return clean
}

// absPath returns an absolute path for a document path stored on a Graph.
// Document paths are relative to g.RootDir; callers that need to touch the
// file on disk (read/write/git) must join with the root first.
func (g *Graph) absPath(docPath string) string {
	if filepath.IsAbs(docPath) {
		return docPath
	}
	return filepath.Join(g.RootDir, docPath)
}

func ParseDocument(path string) (*Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	frontmatter, body, err := ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}
	if !frontmatter.HasFrontmatter || frontmatter.Doc.ID == "" {
		return nil, fmt.Errorf("%w in %s", ErrNotDocGraphDocument, path)
	}

	review := ReviewMetadata{
		SelfHash:   frontmatter.Doc.Review.SelfHash,
		ReviewedAt: frontmatter.Doc.Review.ReviewedAt,
		Deps:       frontmatter.Doc.Review.Deps,
	}
	if review.Deps == nil {
		review.Deps = map[string]string{}
	}

	doc := &Document{
		ID:            frontmatter.Doc.ID,
		Title:         extractTitle([]byte(body)),
		DependsOn:     dedupeSortedStrings(frontmatter.Doc.DependsOn),
		Inputs:        dedupeSortedStrings(frontmatter.Doc.Inputs),
		Review:        review,
		ParkingLot:    frontmatter.Doc.ParkingLot,
		Prompt:        frontmatter.Doc.Prompt,
		ExecDef:       frontmatter.Doc.Exec,
		body:          body,
		bodyLinkTexts: extractBodyLinks(body),
	}
	if frontmatter.Raw != nil {
		doc.frontmatter = frontmatter.Raw
	}

	contentHash, err := HashDocument(frontmatter.Raw, body)
	if err != nil {
		return nil, err
	}
	doc.contentHash = contentHash
	return doc, nil
}

func (g *Graph) buildDependents() {
	g.Dependents = make(map[string][]string)
	for id := range g.Documents {
		g.Dependents[id] = g.Dependents[id][:0]
	}
	for id, doc := range g.Documents {
		for _, dep := range doc.DependsOn {
			if _, ok := g.Documents[dep]; !ok {
				continue
			}
			if containsString(g.Dependents[dep], id) {
				continue
			}
			g.Dependents[dep] = append(g.Dependents[dep], id)
		}
	}
	for id := range g.Dependents {
		g.Dependents[id] = dedupeSortedStrings(g.Dependents[id])
	}
	for id, doc := range g.Documents {
		doc.Dependents = append([]string{}, g.Dependents[id]...)
	}
}

func (g *Graph) detectCycles() {
	index := 0
	indexes := map[string]int{}
	lowlinks := map[string]int{}
	onStack := map[string]bool{}
	stack := []string{}
	warned := map[string]struct{}{}
	var visit func(string)

	visit = func(id string) {
		indexes[id] = index
		lowlinks[id] = index
		index++
		stack = append(stack, id)
		onStack[id] = true

		node := g.Documents[id]
		for _, dep := range node.DependsOn {
			if _, ok := g.Documents[dep]; !ok {
				continue
			}
			if _, ok := indexes[dep]; !ok {
				visit(dep)
				if lowlinks[dep] < lowlinks[id] {
					lowlinks[id] = lowlinks[dep]
				}
			} else if onStack[dep] {
				if indexes[dep] < lowlinks[id] {
					lowlinks[id] = indexes[dep]
				}
			}
		}

		if lowlinks[id] == indexes[id] {
			cycle := []string{}
			for {
				last := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[last] = false
				cycle = append(cycle, last)
				if last == id {
					break
				}
			}
			if len(cycle) > 1 {
				warnKey := cycleKey(cycle)
				if _, done := warned[warnKey]; done {
					return
				}
				warned[warnKey] = struct{}{}
				label := renderCycle(cycle)
				g.Warnings = append(g.Warnings, fmt.Sprintf("cycle detected: %s", label))
			} else {
				parent := g.Documents[id]
				selfDepends := containsString(parent.DependsOn, id)
				if selfDepends {
					label := fmt.Sprintf("%s -> %s", id, id)
					g.Warnings = append(g.Warnings, fmt.Sprintf("cycle detected: %s", label))
				}
			}
		}
	}

	for id := range g.Documents {
		if _, ok := indexes[id]; !ok {
			visit(id)
		}
	}

	sort.Strings(g.Warnings)
}

func cycleKey(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	copyIDs := append([]string(nil), ids...)
	sort.Strings(copyIDs)
	return strings.Join(copyIDs, "::")
}

func renderCycle(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	copyIDs := append([]string(nil), ids...)
	sort.Strings(copyIDs)
	start := copyIDs[0]
	return start + " -> " + strings.Join(copyIDs[1:], " -> ") + " -> " + start
}

func (g *Graph) StaleDocs() []StaleReason {
	if len(g.Documents) == 0 {
		return nil
	}

	direct := map[string][]string{}
	for id := range g.Documents {
		doc, ok := g.Documents[id]
		if !ok || doc.ParkingLot {
			continue
		}
		reasons := staleReasonsForDocument(g, doc)
		if len(reasons) > 0 {
			direct[id] = reasons
		}
	}

	final := map[string][]string{}
	queue := []string{}
	for id, reasons := range direct {
		final[id] = append([]string(nil), reasons...)
		queue = append(queue, id)
	}

	seen := map[string]struct{}{}
	for _, id := range queue {
		seen[id] = struct{}{}
	}
	for i := 0; i < len(queue); i++ {
		u := queue[i]
		for _, dep := range g.Dependents[u] {
			child, ok := g.Documents[dep]
			if !ok || child.ParkingLot {
				continue
			}
			if _, done := seen[dep]; done {
				continue
			}
			seen[dep] = struct{}{}
			final[dep] = []string{"upstream stale dependency"}
			queue = append(queue, dep)
		}
	}

	ids := make([]string, 0, len(final))
	for id := range final {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]StaleReason, 0, len(ids))
	for _, id := range ids {
		doc := g.Documents[id]
		reason := StaleReason{ID: id, Path: doc.Path, Reasons: final[id]}
		sort.Strings(reason.Reasons)
		result = append(result, reason)
	}
	return result
}

func staleReasonsForDocument(g *Graph, doc *Document) []string {
	reasons := []string{}
	for _, depID := range doc.DependsOn {
		depDoc, ok := g.Documents[depID]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("missing dependency: %s", depID))
			continue
		}
		expected := doc.Review.Deps[depID]
		if expected == "" {
			reasons = append(reasons, fmt.Sprintf("missing review hash for dependency: %s", depID))
			continue
		}
		if expected != depDoc.contentHash {
			reasons = append(reasons, fmt.Sprintf("dependency changed: %s", depID))
		}
	}
	return dedupeSortedStrings(reasons)
}

func (g *Graph) StaleReasonForID(id string) (StaleReason, bool) {
	stale := g.StaleDocs()
	for _, entry := range stale {
		if entry.ID == id {
			return entry, true
		}
	}
	doc, ok := g.Documents[id]
	if !ok {
		return StaleReason{}, false
	}
	return StaleReason{ID: id, Path: doc.Path, Reasons: nil}, true
}

func (g *Graph) Dependencies(id string) ([]string, error) {
	doc, ok := g.Documents[id]
	if !ok {
		return nil, fmt.Errorf("document %q not found", id)
	}
	deps := append([]string(nil), doc.DependsOn...)
	sort.Strings(deps)
	return deps, nil
}

func (g *Graph) DependentIDs(id string) ([]string, error) {
	doc, ok := g.Documents[id]
	if !ok {
		return nil, fmt.Errorf("document %q not found", id)
	}
	reqs := append([]string(nil), doc.Dependents...)
	sort.Strings(reqs)
	return reqs, nil
}

func (g *Graph) ResolvePathsToIDs(targets []string) ([]string, error) {
	ids := make([]string, 0, len(targets))
	seen := map[string]struct{}{}
	for _, target := range targets {
		if target == "" {
			continue
		}
		if id, ok := g.Documents[target]; ok {
			if _, done := seen[id.ID]; !done {
				seen[id.ID] = struct{}{}
				ids = append(ids, id.ID)
			}
			continue
		}
		resolved := target
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(g.RootDir, resolved)
		}
		resolved = filepath.Clean(resolved)
		lookupKey := relPath(g.RootDir, resolved)
		if id, ok := g.PathToID[lookupKey]; ok {
			if _, done := seen[id]; !done {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
			continue
		}
		if info, err := os.Stat(resolved); err == nil && info.IsDir() {
			err := filepath.Walk(resolved, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() {
					switch info.Name() {
					case ".git", ".ddx", ".claude", "worktrees":
						return filepath.SkipDir
					}
					return nil
				}
				if strings.EqualFold(filepath.Ext(path), ".md") {
					walkKey := relPath(g.RootDir, path)
					if id, ok := g.PathToID[walkKey]; ok {
						if _, done := seen[id]; !done {
							seen[id] = struct{}{}
							ids = append(ids, id)
						}
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		return nil, fmt.Errorf("cannot resolve target %q", target)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no matching targets")
	}
	return dedupeSortedStrings(ids), nil
}

func (g *Graph) Stamp(targets []string, now time.Time) ([]string, []string, error) {
	docIDs, err := g.ResolvePathsToIDs(targets)
	if err != nil {
		return nil, nil, err
	}

	stamped := make([]string, 0, len(docIDs))
	warnings := []string{}
	for _, id := range docIDs {
		doc, ok := g.Documents[id]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("document %q not found", id))
			continue
		}
		if doc.frontmatter == nil {
			warnings = append(warnings, fmt.Sprintf("document %q missing frontmatter", id))
			continue
		}
		review := ReviewMetadata{
			SelfHash:   doc.contentHash,
			ReviewedAt: now.UTC().Format(time.RFC3339),
			Deps:       map[string]string{},
		}
		for _, depID := range doc.DependsOn {
			if dep, ok := g.Documents[depID]; ok {
				review.Deps[depID] = dep.contentHash
			}
		}
		if review.Deps == nil {
			review.Deps = map[string]string{}
		}
		reviewForWrite := DocReview(review)
		err := SetReview(doc.frontmatter, reviewForWrite)
		if err != nil {
			return stamped, warnings, err
		}
		frontmatterText, err := EncodeFrontmatter(doc.frontmatter)
		if err != nil {
			return stamped, warnings, err
		}
		updated := frontmatterSeparator + "\n" + frontmatterText + "\n" + frontmatterSeparator + "\n" + doc.body
		if err := os.WriteFile(g.absPath(doc.Path), []byte(updated), 0644); err != nil {
			return stamped, warnings, err
		}
		if doc.Review.ReviewedAt == "" || doc.Review.SelfHash != review.SelfHash {
			doc.Review = review
		}
		stamped = append(stamped, id)
	}
	sort.Strings(stamped)
	return stamped, warnings, nil
}

func (g *Graph) Show(id string) (Document, bool) {
	doc, ok := g.Documents[id]
	if !ok {
		return Document{}, false
	}
	return *doc, true
}

func (g *Graph) All() []string {
	ids := make([]string, 0, len(g.Documents))
	for id := range g.Documents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (g *Graph) AllNodesForOutput() []Document {
	ids := g.All()
	out := make([]Document, 0, len(ids))
	for _, id := range ids {
		doc := g.Documents[id]
		if doc == nil {
			continue
		}
		out = append(out, *doc)
	}
	return out
}

func extractTitle(body []byte) string {
	for _, line := range strings.Split(normalizeNewlines(string(body)), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func dedupeSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

var bodyLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// extractBodyLinks returns the raw text inside every [[...]] span in body.
// Nested or malformed brackets are skipped.
func extractBodyLinks(body string) []string {
	matches := bodyLinkPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		text := strings.TrimSpace(m[1])
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// slugifyID converts a wikilink label into a lowercase-hyphen slug suitable
// for ID matching. Dots are preserved (dotted IDs). Spaces and underscores
// become hyphens. Other non-alphanumeric characters are dropped.
func slugifyID(text string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.':
			b.WriteRune(r)
			prevHyphen = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}

// resolveBodyLinks merges body [[ID]] references into each document's DependsOn.
func (g *Graph) resolveBodyLinks() {
	for _, doc := range g.Documents {
		if len(doc.bodyLinkTexts) == 0 {
			continue
		}
		for _, text := range doc.bodyLinkTexts {
			resolved := resolveBodyLinkText(text, g.Documents)
			if resolved == "" || resolved == doc.ID {
				continue
			}
			if !containsString(doc.DependsOn, resolved) {
				doc.DependsOn = append(doc.DependsOn, resolved)
			}
		}
		doc.DependsOn = dedupeSortedStrings(doc.DependsOn)
	}
}

// resolveBodyLinkText tries to resolve a raw wikilink text to a document ID.
// Resolution order: exact match, then slug match.
func resolveBodyLinkText(text string, docs map[string]*Document) string {
	if _, ok := docs[text]; ok {
		return text
	}
	slug := slugifyID(text)
	if slug != text {
		if _, ok := docs[slug]; ok {
			return slug
		}
	}
	return ""
}

func LoadGraphConfigs(workingDir string) ([]GraphConfig, error) {
	cfgDir := filepath.Join(workingDir, ".ddx", "graphs")
	entries, err := os.ReadDir(cfgDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	configs := []GraphConfig{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		cfgPath := filepath.Join(cfgDir, entry.Name())
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("read graph config %q: %w", cfgPath, err)
		}
		var cfg GraphConfig
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parse graph config %q: %w", cfgPath, err)
		}
		configs = append(configs, cfg)
	}
	sort.Slice(configs, func(i, j int) bool {
		return len(configs[i].Roots) > len(configs[j].Roots)
	})
	return configs, nil
}
