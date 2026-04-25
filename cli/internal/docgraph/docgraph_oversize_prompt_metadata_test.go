package docgraph

// docgraph_oversize_prompt_metadata_test.go covers FEAT-022 §9 / Stage D2:
// a docgraph entry whose `prompt:` frontmatter metadata exceeds the
// configured cap must hard-fail the graph build with an actionable error.

import (
	"errors"
	"strings"
	"testing"
)

func TestDocgraphOversizePromptMetadata(t *testing.T) {
	const testCap = 1024
	prev := docgraphPromptCapBytes
	docgraphPromptCapBytes = testCap
	t.Cleanup(func() { docgraphPromptCapBytes = prev })

	huge := strings.Repeat("x", testCap*2)
	root := setupTestRepo(t, map[string]string{
		"docs/oversize.md": "---\nddx:\n  id: oversize.doc\n  prompt: |\n    " + huge + "\n---\n# Oversize\n",
	})

	graph, err := BuildGraph(root)
	if err == nil {
		t.Fatalf("expected oversize error, got graph with %d documents", len(graph.Documents))
	}
	if !errors.Is(err, ErrPromptMetadataOversize) {
		t.Errorf("expected ErrPromptMetadataOversize, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"oversize.doc", "2048", "1024", "evidence_caps.max_prompt_bytes"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}
