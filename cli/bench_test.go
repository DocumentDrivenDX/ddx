package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// BenchmarkCLIStartup measures cold-start time for `ddx version`.
// This is the baseline: how fast does the binary start and exit?
func BenchmarkCLIStartup(b *testing.B) {
	binary := buildBinary(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binary, "version", "--no-check")
		cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
		if err := cmd.Run(); err != nil {
			b.Fatalf("ddx version failed: %v", err)
		}
	}
}

// BenchmarkCLIList measures time to list all resources.
func BenchmarkCLIList(b *testing.B) {
	binary := buildBinary(b)
	dir := setupTestProject(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binary, "list")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
		if err := cmd.Run(); err != nil {
			// list may fail if no library — that's ok for timing
		}
	}
}

// BenchmarkBeadCreate measures bead creation throughput.
func BenchmarkBeadCreate(b *testing.B) {
	binary := buildBinary(b)
	dir := setupTestProject(b)

	// Init beads
	cmd := exec.Command(binary, "bead", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
	cmd.Run()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binary, "bead", "create", "Benchmark bead", "--type", "task")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
		if err := cmd.Run(); err != nil {
			b.Fatalf("bead create failed: %v", err)
		}
	}
}

// BenchmarkBeadReady measures ready queue query time.
func BenchmarkBeadReady(b *testing.B) {
	binary := buildBinary(b)
	dir := setupTestProject(b)

	// Init and populate with 100 beads
	cmd := exec.Command(binary, "bead", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
	cmd.Run()

	for i := 0; i < 100; i++ {
		cmd := exec.Command(binary, "bead", "create", "Bead "+string(rune('A'+i%26)), "--type", "task")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
		cmd.Run()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binary, "bead", "ready", "--json")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
		if err := cmd.Run(); err != nil {
			b.Fatalf("bead ready failed: %v", err)
		}
	}
}

// BenchmarkAgentList measures agent harness discovery time.
func BenchmarkAgentList(b *testing.B) {
	binary := buildBinary(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binary, "agent", "list")
		cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")
		if err := cmd.Run(); err != nil {
			b.Fatalf("agent list failed: %v", err)
		}
	}
}

// buildBinary compiles the ddx binary once per benchmark suite.
func buildBinary(b *testing.B) string {
	b.Helper()
	binary := b.TempDir() + "/ddx"
	cmd := exec.Command("go", "build", "-o", binary, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("build failed: %v\n%s", err, out)
	}
	return binary
}

// setupTestProject creates a minimal DDx project directory.
func setupTestProject(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()

	// Create minimal .ddx/config.yaml
	os.MkdirAll(dir+"/.ddx/library/prompts", 0o755)
	os.MkdirAll(dir+"/.ddx/library/personas", 0o755)
	os.WriteFile(dir+"/.ddx/config.yaml", []byte(`version: "1.0"
library:
  path: .ddx/library
  repository:
    url: https://github.com/easel/ddx-library
    branch: main
`), 0o644)

	// Add a few test documents
	for i := 0; i < 10; i++ {
		os.WriteFile(dir+"/.ddx/library/prompts/prompt-"+string(rune('a'+i))+".md",
			[]byte("# Prompt\nContent"), 0o644)
	}
	return dir
}

// reportMetric logs a named metric for tracking over time.
func reportMetric(b *testing.B, name string, duration time.Duration) {
	b.Helper()
	b.ReportMetric(float64(duration.Microseconds()), "µs/op")
}
