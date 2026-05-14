package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageRatchetReadsBaselineFromConfig(t *testing.T) {
	dir := t.TempDir()

	pathA := filepath.Join(dir, "a.yml")
	if err := os.WriteFile(pathA, []byte("baseline: 42.5\n"), 0o644); err != nil {
		t.Fatalf("write config A: %v", err)
	}
	gotA, err := LoadBaseline(pathA)
	if err != nil {
		t.Fatalf("LoadBaseline(A): %v", err)
	}
	if gotA != 42.5 {
		t.Fatalf("LoadBaseline(A) = %v, want 42.5", gotA)
	}

	// A second config with a different value must produce a different
	// baseline, proving the value is driven by the file rather than a
	// compiled-in fallback.
	pathB := filepath.Join(dir, "b.yml")
	if err := os.WriteFile(pathB, []byte("baseline: 17.3\n"), 0o644); err != nil {
		t.Fatalf("write config B: %v", err)
	}
	gotB, err := LoadBaseline(pathB)
	if err != nil {
		t.Fatalf("LoadBaseline(B): %v", err)
	}
	if gotB != 17.3 {
		t.Fatalf("LoadBaseline(B) = %v, want 17.3", gotB)
	}

	// A missing file must error rather than silently returning a
	// hardcoded default, so the ratchet cannot regress unobserved.
	missing := filepath.Join(dir, "does-not-exist.yml")
	if _, err := LoadBaseline(missing); err == nil {
		t.Fatal("LoadBaseline(missing) returned nil error; expected hard failure (no compiled fallback)")
	}
}

func TestCoverageRatchetFailsBelowBaseline(t *testing.T) {
	err := Enforce(50.0, 60.0)
	if err == nil {
		t.Fatal("Enforce(50, 60) returned nil; want regression error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "regression") {
		t.Fatalf("error %q should mention regression", msg)
	}
	if !strings.Contains(msg, "50.00") || !strings.Contains(msg, "60.00") {
		t.Fatalf("error %q should include measured and baseline values", msg)
	}
}

func TestCoverageRatchetAllowsEqualOrImprovedCoverage(t *testing.T) {
	if err := Enforce(60.0, 60.0); err != nil {
		t.Fatalf("Enforce(60, 60) = %v, want nil (equal coverage must pass)", err)
	}
	if err := Enforce(80.5, 60.0); err != nil {
		t.Fatalf("Enforce(80.5, 60) = %v, want nil (improved coverage must pass)", err)
	}
}

func TestCoverageFromProfileComputesAggregate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.out")
	// 10 statements total, 6 covered -> 60.00%
	contents := strings.Join([]string{
		"mode: set",
		"github.com/example/pkg/a.go:1.1,3.2 4 1",
		"github.com/example/pkg/a.go:5.1,7.2 2 1",
		"github.com/example/pkg/b.go:1.1,4.2 3 0",
		"github.com/example/pkg/b.go:6.1,7.2 1 0",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	got, err := CoverageFromProfile(path)
	if err != nil {
		t.Fatalf("CoverageFromProfile: %v", err)
	}
	if got < 59.99 || got > 60.01 {
		t.Fatalf("CoverageFromProfile = %v, want ~60.0", got)
	}
}
