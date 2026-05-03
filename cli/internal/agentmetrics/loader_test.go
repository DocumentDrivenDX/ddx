package agentmetrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFile materialises a path under workingDir, creating directories as
// needed. Test helper.
func writeFile(t *testing.T, workingDir, rel, body string) {
	t.Helper()
	full := filepath.Join(workingDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestLoadAttempts_BundleOnly exercises the legacy fallback path: only
// .ddx/executions/ exists. Status is bucketed and tier is filled in.
func TestLoadAttempts_BundleOnly(t *testing.T) {
	wd := t.TempDir()
	writeFile(t, wd, ".ddx/executions/20260101T000000-aaa/result.json", `{
		"bead_id":"ddx-bead1","attempt_id":"20260101T000000-aaa",
		"status":"success","outcome":"merged",
		"harness":"claude","provider":"anthropic","model":"sonnet",
		"duration_ms":1000,"cost_usd":0.5,"exit_code":0,
		"started_at":"2026-01-01T00:00:00Z","finished_at":"2026-01-01T00:00:01Z"
	}`)
	writeFile(t, wd, ".ddx/executions/20260101T000100-bbb/result.json", `{
		"bead_id":"ddx-bead2","attempt_id":"20260101T000100-bbb",
		"status":"already_satisfied","harness":"codex","model":"gpt5",
		"duration_ms":500,"cost_usd":0.1,"exit_code":0,
		"started_at":"2026-01-01T00:01:00Z"
	}`)
	writeFile(t, wd, ".ddx/executions/20260101T000200-ccc/result.json", `{
		"bead_id":"ddx-bead3","attempt_id":"20260101T000200-ccc",
		"status":"execution_failed","exit_code":1,"cost_usd":0.2,
		"harness":"claude","model":"haiku",
		"started_at":"2026-01-01T00:02:00Z"
	}`)

	got, err := LoadAttempts(wd)
	if err != nil {
		t.Fatalf("LoadAttempts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(got))
	}

	byID := indexByID(got)

	a1 := byID["20260101T000000-aaa"]
	if a1.Bucket != BucketSuccess {
		t.Fatalf("a1 bucket = %q, want success", a1.Bucket)
	}
	if a1.Tier != "claude/sonnet" {
		t.Fatalf("a1 tier = %q, want claude/sonnet", a1.Tier)
	}
	if a1.Source != SourceBundle {
		t.Fatalf("a1 source = %q, want bundle", a1.Source)
	}

	// already_satisfied counts as success per locked decision.
	a2 := byID["20260101T000100-bbb"]
	if a2.Bucket != BucketSuccess {
		t.Fatalf("already_satisfied must classify as BucketSuccess; got %q", a2.Bucket)
	}
	if !a2.Bucket.Successful() {
		t.Fatalf("already_satisfied bucket must be Successful()")
	}

	a3 := byID["20260101T000200-ccc"]
	if a3.Bucket != BucketExecFailed {
		t.Fatalf("a3 bucket = %q, want exec_failed", a3.Bucket)
	}
	if a3.Bucket.Successful() {
		t.Fatalf("execution_failed must not be Successful()")
	}

	// Sorted ascending by StartedAt.
	if got[0].StartedAt.After(got[1].StartedAt) || got[1].StartedAt.After(got[2].StartedAt) {
		t.Fatalf("attempts must be sorted ascending by started_at")
	}
}

// TestLoadAttempts_MultiSourceDedupe matches Story 8.0 semantics
// (state_runs.go: stored IDs win; bundles only fill gaps). Run-store
// records take precedence over .ddx/executions/ bundles when AttemptID
// collides.
func TestLoadAttempts_MultiSourceDedupe(t *testing.T) {
	wd := t.TempDir()

	// Same attempt id appears in run-store and bundles. Run-store carries
	// authoritative cost; bundle has stale data. Run-store must win.
	writeFile(t, wd, ".ddx/exec/runs/run-A.json", `{
		"id":"20260201T000000-shared","layer":"try",
		"status":"success","outcome":"merged",
		"bead_id":"ddx-bead-A",
		"harness":"claude","provider":"anthropic","model":"sonnet",
		"cost_usd":0.99,"duration_ms":1000,"exit_code":0,
		"started_at":"2026-02-01T00:00:00Z","completed_at":"2026-02-01T00:00:02Z"
	}`)
	writeFile(t, wd, ".ddx/executions/20260201T000000-shared/result.json", `{
		"bead_id":"ddx-bead-A","attempt_id":"20260201T000000-shared",
		"status":"success","cost_usd":0.11,"duration_ms":999,"exit_code":0,
		"harness":"stale","provider":"stale","model":"stale",
		"started_at":"2026-02-01T00:00:00Z"
	}`)

	// Bundle-only attempt is included as a fallback.
	writeFile(t, wd, ".ddx/executions/20260201T000100-bonly/result.json", `{
		"bead_id":"ddx-bead-B","attempt_id":"20260201T000100-bonly",
		"status":"no_changes","exit_code":0,"cost_usd":0.05,
		"harness":"codex","model":"gpt5",
		"started_at":"2026-02-01T00:01:00Z"
	}`)

	// Run-store record at non-try layer must be ignored (run-layer
	// records are per-invocation noise for try-aligned aggregations).
	writeFile(t, wd, ".ddx/exec/runs/run-noise.json", `{
		"id":"sess-noise","layer":"run","status":"success","bead_id":"ddx-bead-A",
		"harness":"claude","cost_usd":0.01,"duration_ms":10
	}`)

	got, err := LoadAttempts(wd)
	if err != nil {
		t.Fatalf("LoadAttempts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped attempts (run-store wins, run-layer ignored, bundle-only included), got %d: %+v", len(got), got)
	}
	byID := indexByID(got)

	shared, ok := byID["20260201T000000-shared"]
	if !ok {
		t.Fatalf("missing shared attempt")
	}
	if shared.Source != SourceRunStore {
		t.Fatalf("collision must resolve to run-store; got source=%q", shared.Source)
	}
	if shared.CostUSD != 0.99 || shared.Harness != "claude" {
		t.Fatalf("run-store fields must win over bundle on collision; got cost=%v harness=%q", shared.CostUSD, shared.Harness)
	}
	if shared.Bucket != BucketSuccess {
		t.Fatalf("shared bucket = %q, want success", shared.Bucket)
	}

	bonly, ok := byID["20260201T000100-bonly"]
	if !ok {
		t.Fatalf("missing bundle-only attempt")
	}
	if bonly.Source != SourceBundle {
		t.Fatalf("bundle-only attempt source = %q, want bundle", bonly.Source)
	}
	if bonly.Bucket != BucketNoChanges {
		t.Fatalf("bundle-only bucket = %q, want no_changes", bonly.Bucket)
	}
}

// TestLoadAttempts_RoutingEnrichment verifies that legacy bundles missing
// harness/provider/model on result.json are enriched from kind:routing
// bead events.
func TestLoadAttempts_RoutingEnrichment(t *testing.T) {
	wd := t.TempDir()
	// Bundle without harness/provider/model.
	writeFile(t, wd, ".ddx/executions/20260301T000000-legacy/result.json", `{
		"bead_id":"ddx-legacy","attempt_id":"20260301T000000-legacy",
		"status":"success","cost_usd":0.4,"duration_ms":2000,"exit_code":0,
		"started_at":"2026-03-01T00:00:00Z"
	}`)

	now := time.Now().UTC().Format(time.RFC3339)
	beadJSON := `{"id":"ddx-legacy","title":"t","status":"closed",` +
		`"issue_type":"task","priority":2,` +
		`"created_at":"` + now + `","updated_at":"` + now + `",` +
		`"events":[{"kind":"routing","summary":"provider=anthropic",` +
		`"body":"{\"resolved_provider\":\"anthropic\",\"resolved_model\":\"sonnet\",\"requested_harness\":\"claude\"}",` +
		`"created_at":"` + now + `"}]}`
	writeFile(t, wd, ".ddx/beads.jsonl", beadJSON+"\n")

	got, err := LoadAttempts(wd)
	if err != nil {
		t.Fatalf("LoadAttempts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(got))
	}
	a := got[0]
	if a.Harness != "claude" {
		t.Fatalf("enriched harness = %q, want claude", a.Harness)
	}
	if a.Provider != "anthropic" {
		t.Fatalf("enriched provider = %q, want anthropic", a.Provider)
	}
	if a.Model != "sonnet" {
		t.Fatalf("enriched model = %q, want sonnet", a.Model)
	}
	if a.Tier != "claude/sonnet" {
		t.Fatalf("enriched tier = %q, want claude/sonnet", a.Tier)
	}
}

// TestLoadAttempts_ExplicitHarnessNotOverwritten ensures a bundle that
// already names a harness is not overwritten by a routing event with a
// different value.
func TestLoadAttempts_ExplicitHarnessNotOverwritten(t *testing.T) {
	wd := t.TempDir()
	writeFile(t, wd, ".ddx/executions/20260401T000000-x/result.json", `{
		"bead_id":"ddx-x","attempt_id":"20260401T000000-x",
		"status":"success","cost_usd":0.1,"duration_ms":1,"exit_code":0,
		"harness":"codex","provider":"openai","model":"gpt5",
		"started_at":"2026-04-01T00:00:00Z"
	}`)
	now := time.Now().UTC().Format(time.RFC3339)
	beadJSON := `{"id":"ddx-x","title":"t","status":"closed",` +
		`"issue_type":"task","priority":2,` +
		`"created_at":"` + now + `","updated_at":"` + now + `",` +
		`"events":[{"kind":"routing","summary":"provider=anthropic",` +
		`"body":"{\"resolved_provider\":\"anthropic\",\"resolved_model\":\"sonnet\",\"requested_harness\":\"claude\"}",` +
		`"created_at":"` + now + `"}]}`
	writeFile(t, wd, ".ddx/beads.jsonl", beadJSON+"\n")

	got, err := LoadAttempts(wd)
	if err != nil {
		t.Fatalf("LoadAttempts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(got))
	}
	a := got[0]
	if a.Harness != "codex" || a.Provider != "openai" || a.Model != "gpt5" {
		t.Fatalf("explicit attempt fields must not be overwritten by routing event; got h=%q p=%q m=%q",
			a.Harness, a.Provider, a.Model)
	}
}

// TestLoadAttempts_EmptyDirs returns an empty slice without error when
// neither source directory exists.
func TestLoadAttempts_EmptyDirs(t *testing.T) {
	wd := t.TempDir()
	got, err := LoadAttempts(wd)
	if err != nil {
		t.Fatalf("LoadAttempts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 attempts, got %d", len(got))
	}
}

func indexByID(in []Attempt) map[string]Attempt {
	out := make(map[string]Attempt, len(in))
	for _, a := range in {
		out[a.AttemptID] = a
	}
	return out
}
