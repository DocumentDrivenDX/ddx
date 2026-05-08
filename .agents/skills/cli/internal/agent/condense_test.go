package agent

import (
	"strings"
	"testing"
)

func TestCondenseOutput_KeepsNamespaceLines(t *testing.T) {
	input := "helix: progress line\nsome verbose noise\nhelix: done\n"
	out := CondenseOutput(input, "helix:")
	if !strings.Contains(out, "helix: progress line") {
		t.Errorf("expected namespace line to be kept, got: %q", out)
	}
	if strings.Contains(out, "verbose noise") {
		t.Errorf("expected verbose noise to be dropped, got: %q", out)
	}
}

func TestCondenseOutput_DropsCodexBoilerplate(t *testing.T) {
	input := "real output\nCommands run:\ncargo test\ncargo build\ntokens used\n500\nmore real output\n"
	out := CondenseOutput(input, "helix:")
	if strings.Contains(out, "Commands run:") {
		t.Errorf("expected 'Commands run:' to be dropped, got: %q", out)
	}
	if strings.Contains(out, "tokens used") {
		t.Errorf("expected 'tokens used' to be dropped, got: %q", out)
	}
	if strings.Contains(out, "500") {
		t.Errorf("expected token count line to be dropped, got: %q", out)
	}
}

func TestCondenseOutput_DropsRawDiffs(t *testing.T) {
	input := "helix: before diff\ndiff --git a/foo.txt b/foo.txt\nindex abc1234..def5678 100644\n--- a/foo.txt\n+++ b/foo.txt\n@@ -1,3 +1,4 @@\n unchanged line\n-removed line\n+added line\n context\nhelix: after diff\n"
	out := CondenseOutput(input, "helix:")
	if strings.Contains(out, "diff --git") {
		t.Errorf("expected diff header to be dropped, got: %q", out)
	}
	if strings.Contains(out, "+added line") {
		t.Errorf("expected diff add line to be dropped, got: %q", out)
	}
	if strings.Contains(out, "-removed line") {
		t.Errorf("expected diff remove line to be dropped, got: %q", out)
	}
	if !strings.Contains(out, "helix: before diff") {
		t.Errorf("expected pre-diff line to be kept, got: %q", out)
	}
	if !strings.Contains(out, "helix: after diff") {
		t.Errorf("expected post-diff line to be kept, got: %q", out)
	}
}

func TestCondenseOutput_KeepsToolCallAndResult(t *testing.T) {
	input := "$ bash tests/helix-cli.sh\nok - test passed\nsome verbose output\n"
	out := CondenseOutput(input, "helix:")
	if !strings.Contains(out, "$ bash tests/helix-cli.sh") {
		t.Errorf("expected tool call to be kept, got: %q", out)
	}
	if !strings.Contains(out, "ok - test passed") {
		t.Errorf("expected tool result line to be kept, got: %q", out)
	}
	if strings.Contains(out, "verbose output") {
		t.Errorf("expected verbose output to be dropped, got: %q", out)
	}
}

func TestCondenseOutput_KeepsErrors(t *testing.T) {
	input := "some noise\nError: something went wrong here\nmore noise\n"
	out := CondenseOutput(input, "helix:")
	if !strings.Contains(out, "Error: something went wrong here") {
		t.Errorf("expected error line to be kept, got: %q", out)
	}
}

func TestCondenseOutput_KeepsMarkdown(t *testing.T) {
	input := "noise\n| **Field** | **Value** |\n|---|---|\n| Status | closed |\nmore noise\n"
	out := CondenseOutput(input, "helix:")
	if !strings.Contains(out, "| **Field** | **Value** |") {
		t.Errorf("expected markdown table to be kept, got: %q", out)
	}
	if !strings.Contains(out, "| Status | closed |") {
		t.Errorf("expected table row to be kept, got: %q", out)
	}
}

func TestCondenseOutput_KeepsIssueIDs(t *testing.T) {
	input := "noise line\nCompleted work on hx-abc12345\nmore noise\n"
	out := CondenseOutput(input, "helix:")
	if !strings.Contains(out, "hx-abc12345") {
		t.Errorf("expected issue ID line to be kept, got: %q", out)
	}
}

func TestCondenseOutput_EmitsAtMostOneBlankBetweenSections(t *testing.T) {
	input := "helix: line1\n\n\n\nhelix: line2\n"
	out := CondenseOutput(input, "helix:")
	// Should not have two consecutive blank lines
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("expected at most one blank line between sections, got: %q", out)
	}
}

func TestCondenseOutput_EmptyInput(t *testing.T) {
	out := CondenseOutput("", "helix:")
	if out != "" {
		t.Errorf("expected empty output for empty input, got: %q", out)
	}
}

func TestCondenseOutput_NoNamespacePrefix(t *testing.T) {
	input := "helix: progress\nError: boom\n"
	out := CondenseOutput(input, "")
	// Without namespace, namespace lines won't match that rule, but errors still kept
	if !strings.Contains(out, "Error: boom") {
		t.Errorf("expected error line kept even without namespace, got: %q", out)
	}
}

func TestCondenseOutput_MixedOutput(t *testing.T) {
	input := `Commands run: 5

helix: [14:24:01] cycle 1: hx-42 (5 ready)

diff --git a/foo.txt b/foo.txt
index abc1234..def5678 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,4 @@
 unchanged line
-removed line
+added line
+another added line
 context

$ bash tests/helix-cli.sh
ok - test passed

**Phase 9 - Output:**

| Field | Value |
|---|---|
| **Selected Issue** | hx-42 |
| **Final Status** | **closed** |

Some random verbose output that should be filtered
More verbose output
Even more verbose output about internal state

helix: cycle 1: hx-42 → COMPLETE

Error: something went wrong here

tokens used
1234
`
	out := CondenseOutput(input, "helix:")

	checks := []struct {
		desc    string
		present bool
		needle  string
	}{
		{"helix progress line", true, "helix: [14:24:01] cycle 1: hx-42 (5 ready)"},
		{"tool call line", true, "$ bash tests/helix-cli.sh"},
		{"tool result line", true, "ok - test passed"},
		{"markdown table row", true, "| **Final Status** | **closed** |"},
		{"completion line", true, "helix: cycle 1: hx-42 → COMPLETE"},
		{"error line", true, "Error: something went wrong here"},
		{"diff header", false, "diff --git"},
		{"diff add", false, "+added line"},
		{"diff remove", false, "-removed line"},
		{"verbose noise", false, "Some random verbose output"},
		{"more verbose noise", false, "More verbose output"},
		{"tokens used", false, "tokens used"},
	}

	for _, c := range checks {
		has := strings.Contains(out, c.needle)
		if c.present && !has {
			t.Errorf("%s: expected %q to be present in output:\n%s", c.desc, c.needle, out)
		}
		if !c.present && has {
			t.Errorf("%s: expected %q to be absent from output:\n%s", c.desc, c.needle, out)
		}
	}
}
