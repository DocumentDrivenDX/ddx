package graphql

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

func newRepairResolver(t *testing.T, dir string) *mutationResolver {
	t.Helper()
	// graphRepairIssue does not touch State, so we can construct the resolver
	// directly without the full StateProvider surface.
	return &mutationResolver{&Resolver{WorkingDir: dir}}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findIssue walks a freshly-built graph and returns the first issue whose kind
// matches; the test fails if no issue is found.
func findIssue(t *testing.T, dir string, kind docgraph.IssueKind) docgraph.GraphIssue {
	t.Helper()
	graph, err := docgraph.BuildGraphWithConfig(dir)
	if err != nil {
		t.Fatalf("BuildGraphWithConfig: %v", err)
	}
	for _, issue := range graph.Issues {
		if issue.Kind == kind {
			return issue
		}
	}
	t.Fatalf("no %s issue in graph; issues=%+v", kind, graph.Issues)
	return docgraph.GraphIssue{}
}

func TestGraphRepairIssue_RemoveMissingDep_Success(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "docs", "alpha.md"),
		"---\nddx:\n  id: alpha\n  depends_on:\n    - beta\n    - ghost\n---\n# Alpha\n")
	writeFile(t, filepath.Join(dir, "docs", "beta.md"),
		"---\nddx:\n  id: beta\n---\n# Beta\n")

	issue := findIssue(t, dir, docgraph.IssueMissingDep)
	id := graphIssueStableID(issue)

	res, err := newRepairResolver(t, dir).GraphRepairIssue(context.Background(), id, RepairStrategyRemoveMissingDep)
	if err != nil {
		t.Fatalf("GraphRepairIssue: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got error=%v", derefStr(res.Error))
	}

	// Confirm the missing dep is gone from the rebuilt graph.
	for _, i := range res.NewIssues {
		if i.Kind == string(docgraph.IssueMissingDep) {
			t.Fatalf("missing_dep issue still present: %+v", i)
		}
	}

	// Confirm the file was actually edited.
	body, err := os.ReadFile(filepath.Join(dir, "docs", "alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "ghost") {
		t.Fatalf("expected ghost dep removed from alpha.md, got:\n%s", body)
	}
	if !strings.Contains(string(body), "beta") {
		t.Fatalf("expected beta dep preserved in alpha.md, got:\n%s", body)
	}
}

func TestGraphRepairIssue_StaleFileHash_Rejected(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "docs", "alpha.md")
	writeFile(t, docPath,
		"---\nddx:\n  id: alpha\n  depends_on:\n    - beta\n    - ghost\n---\n# Alpha\n")
	writeFile(t, filepath.Join(dir, "docs", "beta.md"),
		"---\nddx:\n  id: beta\n---\n# Beta\n")

	issue := findIssue(t, dir, docgraph.IssueMissingDep)
	id := graphIssueStableID(issue)

	// Simulate a concurrent writer that mutates the file between the resolver's
	// initial hash and the pre-write re-hash.
	prev := repairRaceHook
	t.Cleanup(func() { repairRaceHook = prev })
	repairRaceHook = func() {
		writeFile(t, docPath,
			"---\nddx:\n  id: alpha\n  depends_on:\n    - beta\n    - ghost\n    - extra\n---\n# Alpha modified\n")
	}

	res, err := newRepairResolver(t, dir).GraphRepairIssue(context.Background(), id, RepairStrategyRemoveMissingDep)
	if err != nil {
		t.Fatalf("GraphRepairIssue: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure, got success")
	}
	if res.Error == nil || !strings.Contains(*res.Error, "changed") {
		t.Fatalf("expected stale-hash error, got: %v", derefStr(res.Error))
	}
}

func TestGraphRepairIssue_PathTraversal_Rejected(t *testing.T) {
	root := t.TempDir()
	got, err := resolveProjectFile(root, "../etc/passwd")
	if err == nil {
		t.Fatalf("expected path traversal rejection, got: %s", got)
	}
	if !strings.Contains(err.Error(), "escapes project root") {
		t.Fatalf("expected escapes-project-root error, got: %v", err)
	}

	// Absolute path — also rejected.
	if _, err := resolveProjectFile(root, "/etc/passwd"); err == nil {
		t.Fatalf("expected absolute-path rejection")
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
