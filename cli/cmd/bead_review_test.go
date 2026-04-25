package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// beadReviewFixture returns the deterministic inputs used by both the
// golden-file generator and the collapse-equivalence test.
func beadReviewFixture(t *testing.T) (*bead.Bead, []agent.GoverningRef, string, string, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "bead_review"))
	if err != nil {
		t.Fatalf("abs testdata path: %v", err)
	}
	b := &bead.Bead{
		ID:    "ddx-fixture",
		Title: `fixture <title> with "quotes" & 'apos'`,
		Description: "Line one with <tag> & \"quoted\" 'apos'\n" +
			"Line two has > and < and & sentinels.",
		Acceptance: "AC#1: <required> & \"satisfied\"\n" +
			"AC#2: 'covered' > baseline",
		Labels: []string{"kind:refactor", `area:<agent>&"x"`},
	}
	refs := []agent.GoverningRef{
		{ID: "FEAT-XX", Path: "governing.md", Title: `Sample <Doc> & "Title"`},
	}
	rev := "deadbee"
	diff := "commit deadbee\nAuthor: Test <test@example.com>\n\n" +
		"diff --git a/x.go b/x.go\n" +
		"+ added line with <tag> & \"q\" 'a'\n"
	return b, refs, rev, diff, root
}

// TestBeadReviewCollapseGolden is the byte-equivalence gate for FEAT-022 §4
// stage B: agent.BuildReviewPrompt — which the post-collapse CLI handler
// invokes directly — must produce bytes identical to the committed golden
// when run against the fixture inputs. The fixture deliberately includes <,
// >, &, ", and ' so any divergence in the XML-escape semantics surfaces
// here rather than as silent prompt drift.
//
// Regenerate the golden with: UPDATE_GOLDEN=1 go test -run TestBeadReviewCollapseGolden ./cmd/...
func TestBeadReviewCollapseGolden(t *testing.T) {
	b, refs, rev, diff, root := beadReviewFixture(t)
	got := agent.BuildReviewPrompt(b, 1, rev, diff, root, refs)

	goldenPath := filepath.Join("testdata", "bead_review", "collapse_golden.txt")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("review prompt diverged from golden:\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}

	for _, ch := range []string{"<", ">", "&", "\"", "'"} {
		if !strings.Contains(string(want), ch) && !strings.Contains(string(want), xmlEscapeChar(ch)) {
			t.Errorf("golden must contain XML-escape-sensitive char %q (raw or escaped)", ch)
		}
	}
}

func xmlEscapeChar(ch string) string {
	switch ch {
	case "<":
		return "&lt;"
	case ">":
		return "&gt;"
	case "&":
		return "&amp;"
	case "\"":
		return "&#34;"
	case "'":
		return "&#39;"
	}
	return ch
}

// TestBeadReviewCommandWiring confirms the post-collapse CLI handler
// (`ddx bead review <id>`) routes prompt assembly through
// agent.BuildReviewPrompt: it invokes the cobra command end-to-end against
// a real git commit and a real bead store, and asserts the output carries
// the same XML structure that BuildReviewPrompt emits for the same inputs.
func TestBeadReviewCommandWiring(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = tmp
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(tmp, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "x.go")
	runGit("commit", "-q", "-m", "init")
	c := exec.Command("git", "rev-parse", "HEAD")
	c.Dir = tmp
	shaOut, err := c.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	ddxDir := filepath.Join(tmp, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	b := &bead.Bead{
		ID:          "ddx-wire",
		Title:       `wired <title> & "x"`,
		Description: "desc <a> & 'b'",
		Acceptance:  "AC#1 <ok>",
		Extra:       map[string]any{"closing_commit_sha": sha},
	}
	if err := store.Create(b); err != nil {
		t.Fatalf("store create: %v", err)
	}
	// Persist closing_commit_sha through Update so it round-trips via Extra.
	if err := store.Update("ddx-wire", func(bd *bead.Bead) {
		if bd.Extra == nil {
			bd.Extra = map[string]any{}
		}
		bd.Extra["closing_commit_sha"] = sha
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	factory := NewCommandFactory(tmp)
	cmd := factory.newBeadReviewCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"ddx-wire"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute: %v\noutput: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "<bead-review>\n") {
		t.Errorf("output missing <bead-review> opener; got prefix %q", firstLine(out))
	}
	if !strings.Contains(out, `<bead id="ddx-wire" iter=1>`) {
		t.Errorf("output missing wired bead header; got:\n%s", out)
	}
	if !strings.Contains(out, `<diff rev=`+quote(sha)+`>`) {
		t.Errorf("output missing diff section for rev %s; got:\n%s", sha, out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "</bead-review>") {
		t.Errorf("output missing </bead-review> closer; got tail:\n%s", lastLines(out, 3))
	}
}

func quote(s string) string { return `"` + s + `"` }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func lastLines(s string, n int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	return strings.Join(parts, "\n")
}
