package graphql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"gopkg.in/yaml.v3"
)

// repairRaceHook is a test seam invoked between the initial file read and the
// pre-write hash check. Production callers leave it nil; tests assign it to
// simulate a concurrent file modification so the stale-hash branch is
// exercised deterministically.
var repairRaceHook func()

// GraphRepairIssue applies a structured edit for an auto-repairable graph
// integrity issue, then re-validates the graph and returns the new issue list.
// The mutation refuses repairs whose target file changed during the call
// (concurrent repair) or whose target path escapes the project root.
func (r *mutationResolver) GraphRepairIssue(ctx context.Context, issueID string, strategy RepairStrategy) (*GraphRepairResult, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if !strategy.IsValid() {
		return graphRepairFail(r.workingDir(ctx), fmt.Sprintf("unsupported repair strategy %q", strategy)), nil
	}

	graph, err := docgraph.BuildGraphWithConfig(r.workingDir(ctx))
	if err != nil {
		return nil, fmt.Errorf("building document graph: %w", err)
	}

	issue, ok := findIssueByStableID(graph.Issues, issueID)
	if !ok {
		return graphRepairFail(r.workingDir(ctx), "issue not found; the graph may have changed"), nil
	}

	if !strategySupportsKind(strategy, issue.Kind) {
		return graphRepairFail(r.workingDir(ctx), fmt.Sprintf("strategy %s cannot repair issue kind %q", strategy, issue.Kind)), nil
	}

	switch strategy {
	case RepairStrategyRemoveMissingDep:
		if err := repairRemoveMissingDep(graph, issue); err != nil {
			return graphRepairFail(r.workingDir(ctx), err.Error()), nil
		}
	case RepairStrategyApplySuggestedID:
		if err := repairApplySuggestedID(graph, issue); err != nil {
			return graphRepairFail(r.workingDir(ctx), err.Error()), nil
		}
	case RepairStrategyCleanPathMap:
		if err := repairCleanPathMap(r.workingDir(ctx), issue); err != nil {
			return graphRepairFail(r.workingDir(ctx), err.Error()), nil
		}
	default:
		return graphRepairFail(r.workingDir(ctx), fmt.Sprintf("unsupported repair strategy %q", strategy)), nil
	}

	rebuilt, err := docgraph.BuildGraphWithConfig(r.workingDir(ctx))
	if err != nil {
		return nil, fmt.Errorf("rebuilding graph after repair: %w", err)
	}
	return &GraphRepairResult{
		Success:   true,
		NewIssues: issuesToGQL(rebuilt.Issues),
	}, nil
}

func graphRepairFail(workingDir, msg string) *GraphRepairResult {
	out := &GraphRepairResult{
		Success:   false,
		NewIssues: []*GraphIssue{},
		Error:     &msg,
	}
	if workingDir != "" {
		if g, err := docgraph.BuildGraphWithConfig(workingDir); err == nil {
			out.NewIssues = issuesToGQL(g.Issues)
		}
	}
	return out
}

func findIssueByStableID(issues []docgraph.GraphIssue, id string) (docgraph.GraphIssue, bool) {
	for _, issue := range issues {
		if graphIssueStableID(issue) == id {
			return issue, true
		}
	}
	return docgraph.GraphIssue{}, false
}

func strategySupportsKind(strategy RepairStrategy, kind docgraph.IssueKind) bool {
	switch strategy {
	case RepairStrategyRemoveMissingDep:
		return kind == docgraph.IssueMissingDep
	case RepairStrategyApplySuggestedID:
		return kind == docgraph.IssueDuplicateID
	case RepairStrategyCleanPathMap:
		return kind == docgraph.IssueIDPathMissing || kind == docgraph.IssueIDPathMismatch
	}
	return false
}

// resolveProjectFile validates that relPath stays under root and returns its
// absolute, cleaned form. It rejects absolute paths and any path whose
// resolved form escapes root.
func resolveProjectFile(root, relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is empty")
	}
	clean := filepath.Clean(filepath.FromSlash(relPath))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("path must be relative to project root")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root")
	}
	abs := filepath.Clean(filepath.Join(root, clean))
	rootAbs := filepath.Clean(root)
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes project root")
	}
	return abs, nil
}

// readAndCheckStable reads the file at abs and confirms its content hash has
// not drifted from the snapshot captured at graph-build time. The repairRaceHook
// test seam fires in between to simulate a concurrent writer.
func readAndCheckStable(absPath, expectedHash string) (*yaml.Node, string, error) {
	currentHash, err := docgraph.HashDocumentFile(absPath)
	if err != nil {
		return nil, "", fmt.Errorf("hash file: %w", err)
	}
	if expectedHash != "" && currentHash != expectedHash {
		return nil, "", fmt.Errorf("file has changed since the issue was computed; refresh the page")
	}
	if repairRaceHook != nil {
		repairRaceHook()
	}
	rehash, err := docgraph.HashDocumentFile(absPath)
	if err != nil {
		return nil, "", fmt.Errorf("rehash file: %w", err)
	}
	if rehash != currentHash {
		return nil, "", fmt.Errorf("file changed during repair; aborting")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, "", err
	}
	fm, body, err := docgraph.ParseFrontmatter(content)
	if err != nil {
		return nil, "", err
	}
	if !fm.HasFrontmatter || fm.Raw == nil {
		return nil, "", fmt.Errorf("file has no frontmatter")
	}
	return fm.Raw, body, nil
}

func repairRemoveMissingDep(graph *docgraph.Graph, issue docgraph.GraphIssue) error {
	if issue.Path == "" || issue.ID == "" {
		return fmt.Errorf("missing_dep issue is incomplete")
	}
	abs, err := resolveProjectFile(graph.RootDir, issue.Path)
	if err != nil {
		return err
	}
	declaringID, ok := graph.PathToID[issue.Path]
	if !ok {
		return fmt.Errorf("declaring document for %q is not in the graph", issue.Path)
	}
	doc := graph.Documents[declaringID]
	if doc == nil {
		return fmt.Errorf("declaring document for %q is not in the graph", issue.Path)
	}
	root, body, err := readAndCheckStable(abs, doc.ContentHash())
	if err != nil {
		return err
	}
	ns := findNamespaceNode(root)
	if ns == nil {
		return fmt.Errorf("frontmatter has no ddx/dun namespace")
	}
	depsNode := findChild(ns, "depends_on")
	if depsNode == nil || depsNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("depends_on missing or not a sequence")
	}
	target := strings.TrimSpace(issue.ID)
	filtered := depsNode.Content[:0]
	removed := false
	for _, n := range depsNode.Content {
		if n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) == target {
			removed = true
			continue
		}
		filtered = append(filtered, n)
	}
	if !removed {
		return fmt.Errorf("dependency %q not found in depends_on", target)
	}
	depsNode.Content = filtered
	if len(depsNode.Content) == 0 {
		removeChild(ns, "depends_on")
	}
	return writeFrontmatter(abs, root, body)
}

func repairApplySuggestedID(graph *docgraph.Graph, issue docgraph.GraphIssue) error {
	if issue.Path == "" || issue.ID == "" {
		return fmt.Errorf("duplicate_id issue is incomplete")
	}
	if issue.RelatedPath == "" {
		return fmt.Errorf("duplicate_id issue has no related path")
	}
	abs, err := resolveProjectFile(graph.RootDir, issue.Path)
	if err != nil {
		return err
	}
	if hasInboundReferences(graph, issue.ID, issue.Path) {
		return fmt.Errorf("duplicate has inbound references; ambiguous and not auto-repairable")
	}
	suggested := docgraph.SuggestUniqueID(issue.ID, issue.Path)

	// The duplicate file is intentionally absent from graph.Documents (the
	// canonical winner took the slot), so we have no graph-recorded hash to
	// compare against. Instead we hash the file twice with the test hook in
	// between to detect concurrent writers.
	root, body, err := readAndCheckStable(abs, "")
	if err != nil {
		return err
	}
	ns := findNamespaceNode(root)
	if ns == nil {
		return fmt.Errorf("frontmatter has no ddx/dun namespace")
	}
	idNode := findChild(ns, "id")
	if idNode == nil || idNode.Kind != yaml.ScalarNode {
		return fmt.Errorf("id field missing")
	}
	idNode.Value = suggested
	idNode.Tag = "!!str"
	return writeFrontmatter(abs, root, body)
}

// hasInboundReferences returns true if any document in the graph declares a
// depends_on edge to the duplicate ID. The rename is unsafe in that case
// because the existing reference expects the ID to keep its current target.
func hasInboundReferences(graph *docgraph.Graph, dupID, dupPath string) bool {
	for _, doc := range graph.Documents {
		if doc == nil || doc.Path == dupPath {
			continue
		}
		for _, dep := range doc.DependsOn {
			if dep == dupID {
				return true
			}
		}
	}
	return false
}

func repairCleanPathMap(workingDir string, issue docgraph.GraphIssue) error {
	if issue.ID == "" {
		return fmt.Errorf("id_to_path issue has no id")
	}
	cfgDir := filepath.Join(workingDir, ".ddx", "graphs")
	entries, err := os.ReadDir(cfgDir)
	if err != nil {
		return fmt.Errorf("read graph config dir: %w", err)
	}
	target := strings.TrimSpace(issue.ID)
	cleaned := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		path := filepath.Join(cfgDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var root yaml.Node
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
			continue
		}
		mapNode := findChild(root.Content[0], "id_to_path")
		if mapNode == nil || mapNode.Kind != yaml.MappingNode {
			continue
		}
		newContent := mapNode.Content[:0]
		removed := false
		for i := 0; i < len(mapNode.Content); i += 2 {
			k := mapNode.Content[i]
			if k != nil && k.Kind == yaml.ScalarNode && strings.TrimSpace(k.Value) == target {
				removed = true
				continue
			}
			newContent = append(newContent, mapNode.Content[i], mapNode.Content[i+1])
		}
		if !removed {
			continue
		}
		mapNode.Content = newContent
		out, err := yaml.Marshal(&root)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, out, 0644); err != nil {
			return err
		}
		cleaned = true
	}
	if !cleaned {
		return fmt.Errorf("id %q not found in id_to_path of any graph config", target)
	}
	return nil
}

// findNamespaceNode returns the ddx: or dun: child mapping (preferring ddx).
func findNamespaceNode(root *yaml.Node) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	if n := findChild(root, "ddx"); n != nil {
		return n
	}
	return findChild(root, "dun")
}

func findChild(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		if k != nil && k.Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func removeChild(node *yaml.Node, key string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		if k != nil && k.Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

func writeFrontmatter(absPath string, root *yaml.Node, body string) error {
	encoded, err := docgraph.EncodeFrontmatter(root)
	if err != nil {
		return err
	}
	updated := "---\n" + encoded + "\n---\n" + body
	return os.WriteFile(absPath, []byte(updated), 0644)
}
