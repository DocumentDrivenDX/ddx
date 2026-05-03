package checks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newCtx(t *testing.T) (string, InvocationContext) {
	t.Helper()
	root := t.TempDir()
	ev := filepath.Join(root, "evidence")
	return root, InvocationContext{
		BeadID:      "ddx-test-1",
		DiffBase:    "BASE",
		DiffHead:    "HEAD",
		ProjectRoot: root,
		EvidenceDir: ev,
		RunID:       "run-1",
	}
}

func TestRun_PassWritesResult(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "writer",
		When:    HookPreMerge,
		Command: `printf '{"status":"pass","message":"ok"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	results, err := Run(context.Background(), []Check{c}, ictx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Status != StatusPass {
		t.Fatalf("status = %s", results[0].Status)
	}
	if results[0].Message != "ok" {
		t.Fatalf("message = %q", results[0].Message)
	}
}

func TestRun_BlockStatusFromFile(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "blocker",
		When:    HookPreMerge,
		Command: `printf '{"status":"block","message":"nope","violations":[{"file":"a.go","line":3,"detail":"x"}]}' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	results, _ := Run(context.Background(), []Check{c}, ictx)
	if results[0].Status != StatusBlock {
		t.Fatalf("status = %s", results[0].Status)
	}
	if len(results[0].Violations) != 1 || results[0].Violations[0].File != "a.go" {
		t.Fatalf("violations = %+v", results[0].Violations)
	}
}

func TestRun_NonZeroExitIsError(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "exiter",
		When:    HookPreMerge,
		Command: `printf '{"status":"pass"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"; exit 7`,
	}
	results, _ := Run(context.Background(), []Check{c}, ictx)
	if results[0].Status != StatusError {
		t.Fatalf("status = %s, want error", results[0].Status)
	}
	if results[0].ExitCode != 7 {
		t.Fatalf("exit = %d", results[0].ExitCode)
	}
}

func TestRun_MissingResultFileIsError(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "silent",
		When:    HookPreMerge,
		Command: `true`,
	}
	results, _ := Run(context.Background(), []Check{c}, ictx)
	if results[0].Status != StatusError {
		t.Fatalf("status = %s, want error", results[0].Status)
	}
	if !strings.Contains(results[0].Message, "no result file") {
		t.Fatalf("message = %q", results[0].Message)
	}
}

func TestRun_InvalidJSONIsError(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "garbage",
		When:    HookPreMerge,
		Command: `printf 'not json' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	results, _ := Run(context.Background(), []Check{c}, ictx)
	if results[0].Status != StatusError {
		t.Fatalf("status = %s, want error", results[0].Status)
	}
}

func TestRun_UnknownStatusIsError(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "weird",
		When:    HookPreMerge,
		Command: `printf '{"status":"maybe"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	results, _ := Run(context.Background(), []Check{c}, ictx)
	if results[0].Status != StatusError {
		t.Fatalf("status = %s, want error", results[0].Status)
	}
}

func TestRun_EnvInjection(t *testing.T) {
	_, ictx := newCtx(t)
	c := Check{
		Name:    "envcheck",
		When:    HookPreMerge,
		Command: `printf '{"status":"pass","message":"BEAD=%s BASE=%s HEAD=%s RUN=%s"}' "$BEAD_ID" "$DIFF_BASE" "$DIFF_HEAD" "$RUN_ID" > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	results, err := Run(context.Background(), []Check{c}, ictx)
	if err != nil {
		t.Fatal(err)
	}
	want := "BEAD=ddx-test-1 BASE=BASE HEAD=HEAD RUN=run-1"
	if results[0].Message != want {
		t.Fatalf("message = %q, want %q", results[0].Message, want)
	}
}

func TestRun_AppliesToFiltering(t *testing.T) {
	_, ictx := newCtx(t)
	ictx.BeadLabels = []string{"area:foo"}
	yes := Check{
		Name: "yes", When: HookPreMerge,
		AppliesTo: AppliesTo{Labels: []string{"area:foo"}},
		Command:   `printf '{"status":"pass"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	no := Check{
		Name: "no", When: HookPreMerge,
		AppliesTo: AppliesTo{Labels: []string{"area:other"}},
		Command:   `printf '{"status":"pass"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
	}
	results, err := Run(context.Background(), []Check{yes, no}, ictx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "yes" {
		t.Fatalf("want only 'yes', got %+v", results)
	}
}

func TestRun_ParallelExecution(t *testing.T) {
	// Two checks that each sleep then write a unique file. Wall time
	// should be close to one sleep, not two — verifying parallelism.
	_, ictx := newCtx(t)
	mk := func(name string) Check {
		return Check{
			Name: name, When: HookPreMerge,
			Command: `sleep 0.4; printf '{"status":"pass"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"`,
		}
	}
	checks := []Check{mk("a"), mk("b"), mk("c")}
	start := time.Now()
	results, err := Run(context.Background(), checks, ictx)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// Sequential would be ~1.2s; parallel should be well under 1s.
	if elapsed > 1000 {
		t.Fatalf("checks ran sequentially? elapsed=%dms", elapsed)
	}
}

func TestRun_StaleResultFileIsCleaned(t *testing.T) {
	_, ictx := newCtx(t)
	if err := os.MkdirAll(ictx.EvidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(ictx.EvidenceDir, "ghost.json")
	if err := os.WriteFile(stale, []byte(`{"status":"pass"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := Check{Name: "ghost", When: HookPreMerge, Command: "true"}
	results, _ := Run(context.Background(), []Check{c}, ictx)
	if results[0].Status != StatusError {
		t.Fatalf("stale result file should be cleaned and run reported as error; got %s", results[0].Status)
	}
}
