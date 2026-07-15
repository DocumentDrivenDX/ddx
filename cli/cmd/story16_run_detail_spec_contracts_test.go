package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStory16RunDetailSpecContracts(t *testing.T) {
	readSpec := func(rel string) string {
		t.Helper()

		path := filepath.Join("..", "..", filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		require.NoError(t, err, "read %s", rel)
		return string(data)
	}

	feat008 := readSpec("docs/helix/01-frame/features/FEAT-008-web-ui.md")
	for _, want := range []string{
		"anyone with project read access can inspect the raw prompt",
		"response, stderr, and tool-call arguments",
		"Evidence browser whitelist",
		"the Evidence tab renders only",
		"`*.txt`",
		"`*.md`",
		"`manifest.json`",
		"`prompt.md`",
		"`result.json`",
		"64 KB or smaller",
		"does not render Markdown as HTML",
	} {
		if !strings.Contains(feat008, want) {
			t.Fatalf("FEAT-008 missing %q", want)
		}
	}

	feat010 := readSpec("docs/helix/01-frame/features/FEAT-010-task-execution.md")
	for _, want := range []string{
		"run(id:)",
		"`prompt`",
		"`response`",
		"`stderr`",
		"`bundleFiles[]`",
		"Tool/session detail is a Fizeau-owned opaque attachment",
		"does not construct a canonical `ToolCallEntry`, parse native logs",
		"typed paginated tool API",
		"added to CONTRACT-003",
		"runBundleFile(id:, path:)",
		"run's bundle root",
		"path traversal",
		"absolute paths",
		"symlink escapes",
	} {
		if !strings.Contains(feat010, want) {
			t.Fatalf("FEAT-010 missing %q", want)
		}
	}

	feat019 := readSpec("docs/helix/01-frame/features/FEAT-019-agent-evaluation.md")
	for _, want := range []string{
		"project-scoped run-detail surface at `/runs/[runId]` remains canonical",
		"FEAT-019 adds evaluation context and links; it does not duplicate run",
		"lifecycle, transcript, or access-control behavior",
	} {
		if !strings.Contains(feat019, want) {
			t.Fatalf("FEAT-019 missing %q", want)
		}
	}
}
