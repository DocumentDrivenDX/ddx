package evidence

import (
	"strings"
	"testing"
)

const sampleDiff = `diff --git a/foo.go b/foo.go
index abc..def 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 line1
+added
 line2
 line3
diff --git a/bar.go b/bar.go
index 111..222 100644
--- a/bar.go
+++ b/bar.go
@@ -10,2 +10,3 @@
 x
+y
 z
`

func TestDecomposeDiff(t *testing.T) {
	files := DecomposeDiff(sampleDiff)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].Path != "foo.go" {
		t.Errorf("file 0 path = %q", files[0].Path)
	}
	if len(files[0].HunkHeaders) != 1 {
		t.Errorf("file 0 hunk count = %d", len(files[0].HunkHeaders))
	}
	if files[1].Path != "bar.go" {
		t.Errorf("file 1 path = %q", files[1].Path)
	}
}

func TestClampDiffWithinBudget(t *testing.T) {
	out, sec := ClampDiff(sampleDiff, len(sampleDiff)+10)
	if out != sampleDiff {
		t.Errorf("expected unchanged")
	}
	if sec.TruncationReason != "" {
		t.Errorf("unexpected truncation: %q", sec.TruncationReason)
	}
}

func TestClampDiffDegrades(t *testing.T) {
	// budget too small for full bodies but enough for degraded form.
	out, sec := ClampDiff(sampleDiff, 200)
	if !strings.Contains(out, "diff --git") {
		t.Errorf("degraded output missing diff header")
	}
	if !strings.Contains(out, "@@ ") {
		t.Errorf("degraded output missing hunk header")
	}
	if sec.TruncationReason == "" {
		t.Errorf("expected non-empty TruncationReason")
	}
}

func TestClampDiffOmitsWhenNoRoom(t *testing.T) {
	out, sec := ClampDiff(sampleDiff, 5)
	if len(out) > 5+len(TruncationMarker) {
		t.Errorf("output exceeds budget by more than marker: %d", len(out))
	}
	_ = sec
}
