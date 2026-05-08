package perf

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestGraphQLPerfMatrix_Baseline is the "measurement surface" the bead calls
// out: it seeds the default fixture, runs every Target, and — when
// DDX_BENCH_REPORT_DIR is set — writes the baseline report into that
// directory. This test is deliberately a `TestXxx` (not a `BenchmarkXxx`) so
// it runs under plain `go test` and anyone can reproduce the numbers with the
// same recipe CI will use.
//
// The acceptance criteria (ddx-9ce6842a AC §5) give per-target p95 budgets.
// This test asserts them for `bead(id:)` and `beadsByProject`; the cross-
// project `beads` target is logged but not gated, because it is the
// worst-case scan shape — a hard assertion here would tie the test to
// hardware. The markdown/JSON baseline preserves those numbers so a CI gate
// in a follow-up bead can diff against them.
func TestGraphQLPerfMatrix_Baseline(t *testing.T) {
	if testing.Short() {
		t.Skip("perf matrix skipped in -short mode")
	}
	spec := DefaultBeadFixtureSpec()
	f := BuildBeadFixture(t, spec)
	report := RunMatrix(t, f, DefaultIterations)

	for _, r := range report.Targets {
		switch r.Name {
		case "bead":
			if r.InProcess.P95 > 50 {
				t.Errorf("bead(id:) in-process p95=%.2fms > 50ms budget", r.InProcess.P95)
			}
			if r.HTTP != nil && r.HTTP.P95 > 200 {
				t.Errorf("bead(id:) HTTP p95=%.2fms > 200ms budget", r.HTTP.P95)
			}
		case "beadsByProject":
			if r.InProcess.P95 > 75 {
				t.Errorf("beadsByProject in-process p95=%.2fms > 75ms budget", r.InProcess.P95)
			}
			if r.HTTP != nil && r.HTTP.P95 > 250 {
				t.Errorf("beadsByProject HTTP p95=%.2fms > 250ms budget", r.HTTP.P95)
			}
		}
		httpLine := "n/a"
		if r.HTTP != nil {
			httpLine = fmt.Sprintf("p50=%.2fms p95=%.2fms p99=%.2fms", r.HTTP.P50, r.HTTP.P95, r.HTTP.P99)
		}
		t.Logf("target=%s iter=%d in-proc(p50=%.2fms p95=%.2fms p99=%.2fms) http(%s)",
			r.Name, r.Iterations,
			r.InProcess.P50, r.InProcess.P95, r.InProcess.P99,
			httpLine,
		)
	}

	// Optional report writing — keeps CI quick by default, lets humans
	// regenerate the baseline with `DDX_BENCH_REPORT_DIR=... go test ...`.
	if dir := os.Getenv("DDX_BENCH_REPORT_DIR"); dir != "" {
		date := os.Getenv("DDX_BENCH_REPORT_DATE")
		if date == "" {
			date = time.Now().UTC().Format("2006-01-02")
		}
		md, js, err := WriteReports(dir, date, report)
		if err != nil {
			t.Fatalf("write reports: %v", err)
		}
		t.Logf("wrote baseline report: %s (+ %s)", md, js)
	}
}

// BenchmarkGraphQLPerfMatrix is the `go test -bench` entry point. Each
// iteration runs the whole matrix so benchstat can diff runs.
func BenchmarkGraphQLPerfMatrix(b *testing.B) {
	spec := DefaultBeadFixtureSpec()
	f := BuildBeadFixture(b, spec)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RunMatrix(b, f, DefaultIterations)
	}
}
