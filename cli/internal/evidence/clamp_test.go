package evidence

import (
	"strings"
	"testing"
)

func TestClampOutputUnderCap(t *testing.T) {
	out, truncated, orig := ClampOutput("hello", 10)
	if truncated {
		t.Fatal("expected not truncated")
	}
	if out != "hello" {
		t.Errorf("got %q, want hello", out)
	}
	if orig != 5 {
		t.Errorf("originalBytes = %d, want 5", orig)
	}
}

func TestClampOutputOverCap(t *testing.T) {
	in := strings.Repeat("x", 1000)
	out, truncated, orig := ClampOutput(in, 200)
	if !truncated {
		t.Fatal("expected truncated")
	}
	if orig != 1000 {
		t.Errorf("originalBytes = %d, want 1000", orig)
	}
	if !strings.HasSuffix(out, TruncationMarker) {
		t.Errorf("expected output to end with TruncationMarker, got %q", out)
	}
	if len(out) > 200 {
		t.Errorf("output len %d exceeds cap 200", len(out))
	}
}

func TestClampOutputMarkerOnly(t *testing.T) {
	in := strings.Repeat("x", 100)
	out, truncated, _ := ClampOutput(in, 5)
	if !truncated {
		t.Fatal("expected truncated")
	}
	if len(out) != 5 {
		t.Errorf("output len %d, want 5", len(out))
	}
}

func TestTruncationMarkerStable(t *testing.T) {
	if TruncationMarker == "" {
		t.Fatal("TruncationMarker must be a non-empty exported constant")
	}
}
