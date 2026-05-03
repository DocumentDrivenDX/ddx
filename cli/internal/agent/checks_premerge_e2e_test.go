package agent

// checks_premerge_e2e_test.go — end-to-end coverage for the pre-merge checks
// gate wired into the execute-bead loop's land-back step.
//
// Each test boots a real temp git repo + .ddx tracker store, drops a fixture
// .ddx/checks/dummy-fail.yaml whose script always returns status=block, and
// drives SubmitWithPreMergeChecks against a synthesised worker
// ExecuteBeadResult to assert the documented contract:
//
//	Test 1 — block fixture causes preserve, submit not called, evidence and
//	         events recorded.
//	Test 2 — block fixture + checks_bypass annotation causes submit to run
//	         and a checks-bypass event to be recorded.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// preMergeRepo is a focused version of landTestRepo specialised for these
// pre-merge gate tests: it owns a .ddx tracker store as well as the git repo,
// so we can create beads and read events without any extra setup.
type preMergeRepo struct {
	t       *testing.T
	dir     string
	store   *bead.Store
	baseSHA string
}

func newPreMergeRepo(t *testing.T) *preMergeRepo {
	t.Helper()
	dir := t.TempDir()
	r := &preMergeRepo{t: t, dir: dir}
	r.runGit("init", "-b", "main")
	r.runGit("config", "user.name", "Test")
	r.runGit("config", "user.email", "test@test.local")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# pre-merge\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.runGit("add", "-A")
	r.runGit("commit", "-m", "init")
	r.baseSHA = strings.TrimSpace(r.runGit("rev-parse", "HEAD"))

	r.store = bead.NewStore(filepath.Join(dir, ".ddx"))
	if err := r.store.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return r
}

func (r *preMergeRepo) runGit(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
	return string(out)
}

// commitNoOp creates a one-file commit reachable via the object store but not
// any branch, returning its SHA. Used as the worker's resultRev so the gate
// has a real (baseRev → resultRev) range to diff against.
func (r *preMergeRepo) commitNoOp() string {
	r.t.Helper()
	wt, err := os.MkdirTemp("", "premerge-wt-*")
	if err != nil {
		r.t.Fatal(err)
	}
	_ = os.RemoveAll(wt)
	r.runGit("worktree", "add", "--detach", wt, r.baseSHA)
	defer func() {
		r.runGit("worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}()
	if err := os.WriteFile(filepath.Join(wt, "noop.txt"), []byte("hello\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", wt, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git add: %s: %v", string(out), err)
	}
	cmd = exec.Command("git", "-C", wt,
		"-c", "user.name=Test", "-c", "user.email=test@test.local",
		"commit", "-m", "noop")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git commit: %s: %v", string(out), err)
	}
	cmd = exec.Command("git", "-C", wt, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git rev-parse: %s: %v", string(out), err)
	}
	return strings.TrimSpace(string(out))
}

// writeDummyFailCheck drops a deterministic check definition that always
// writes status=block and exits 0 (so the runner's failure mode is "result
// file says block", not "command crashed").
func (r *preMergeRepo) writeDummyFailCheck() {
	r.t.Helper()
	checkDir := filepath.Join(r.dir, ".ddx", "checks")
	if err := os.MkdirAll(checkDir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	yaml := `name: dummy-fail
when: pre_merge
command: |
  cat > "${EVIDENCE_DIR}/${CHECK_NAME}.json" <<EOF
  {"status":"block","message":"dummy-fail fixture: always blocks"}
  EOF
`
	if err := os.WriteFile(filepath.Join(checkDir, "dummy-fail.yaml"), []byte(yaml), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// makeBead creates an open bead in the tracker and returns it. extra is
// merged into bead.Extra so callers can attach a checks_bypass annotation.
func (r *preMergeRepo) makeBead(id string, extra map[string]any) *bead.Bead {
	r.t.Helper()
	b := &bead.Bead{
		ID:     id,
		Title:  "fixture bead for pre-merge checks",
		Status: bead.StatusOpen,
		Labels: []string{"phase:2", "area:checks"},
		Extra:  extra,
	}
	if err := r.store.Create(b); err != nil {
		r.t.Fatalf("create bead: %v", err)
	}
	got, err := r.store.Get(id)
	if err != nil {
		r.t.Fatalf("get bead: %v", err)
	}
	return got
}

// readEvents reads the bead's appended events via the store's Events helper.
func (r *preMergeRepo) readEvents(id string) []bead.BeadEvent {
	r.t.Helper()
	evs, err := r.store.Events(id)
	if err != nil {
		r.t.Fatalf("read events: %v", err)
	}
	return evs
}

// TestSubmitWithPreMergeChecks_DummyFailAbortsMerge covers AC #4: a fixture
// dummy-fail check causes merge abort, the worker's resultRev is preserved
// under refs/ddx/iterations/, the per-check evidence file is on disk, and a
// checks-blocked event is appended to the bead.
func TestSubmitWithPreMergeChecks_DummyFailAbortsMerge(t *testing.T) {
	r := newPreMergeRepo(t)
	r.writeDummyFailCheck()
	b := r.makeBead("pmc-block-001", nil)
	resultSHA := r.commitNoOp()

	runID := time.Now().UTC().Format("20060102T150405") + "-pmcblk1"
	res := &ExecuteBeadResult{
		BeadID:       b.ID,
		BaseRev:      r.baseSHA,
		ResultRev:    resultSHA,
		ExecutionDir: filepath.Join(".ddx", "executions", runID),
	}

	submitCalled := false
	submit := func(req LandRequest) (*LandResult, error) {
		submitCalled = true
		return &LandResult{Status: "landed", NewTip: req.ResultRev}, nil
	}

	land, outcome, err := SubmitWithPreMergeChecks(
		context.Background(), r.dir, b, res,
		submit, r.store, "ddx-test-actor", "test-source", nil,
	)
	if err != nil {
		t.Fatalf("SubmitWithPreMergeChecks: %v", err)
	}
	if submitCalled {
		t.Fatalf("submit must NOT be called when pre-merge checks block (AC #4)")
	}
	if outcome == nil || !outcome.Blocked {
		t.Fatalf("outcome must report Blocked=true; got %+v", outcome)
	}
	if !sliceContains(outcome.BlockingNames, "dummy-fail") {
		t.Fatalf("BlockingNames must contain dummy-fail; got %v", outcome.BlockingNames)
	}
	if land == nil || land.Status != "preserved" {
		t.Fatalf("LandResult must be preserved; got %+v", land)
	}
	if land.PreserveRef == "" || !strings.HasPrefix(land.PreserveRef, "refs/ddx/iterations/"+b.ID+"/") {
		t.Fatalf("PreserveRef must follow refs/ddx/iterations/<bead-id>/ convention; got %q", land.PreserveRef)
	}

	// Worktree-preservation invariant (AC #2 + #6): the preserve ref must
	// resolve to the worker's resultSHA in the real git object store.
	got := strings.TrimSpace(r.runGit("rev-parse", land.PreserveRef))
	if got != resultSHA {
		t.Fatalf("preserve ref resolves to %s; want resultSHA %s", got, resultSHA)
	}

	// Evidence file (AC #6): checks runner must have written a JSON result
	// file at <projectRoot>/<execDir>/checks/dummy-fail.json.
	evidenceFile := filepath.Join(r.dir, res.ExecutionDir, "checks", "dummy-fail.json")
	data, readErr := os.ReadFile(evidenceFile)
	if readErr != nil {
		t.Fatalf("evidence file missing at %s: %v", evidenceFile, readErr)
	}
	if !strings.Contains(string(data), `"status":"block"`) {
		t.Fatalf("evidence file does not show status=block: %s", string(data))
	}

	// Event recorded (AC #4): one checks-blocked event for dummy-fail.
	evs := r.readEvents(b.ID)
	var blockedEvents []bead.BeadEvent
	for _, e := range evs {
		if e.Kind == "checks-blocked" {
			blockedEvents = append(blockedEvents, e)
		}
	}
	if len(blockedEvents) != 1 {
		t.Fatalf("expected exactly one checks-blocked event; got %d (events=%+v)", len(blockedEvents), evs)
	}
	if !strings.Contains(blockedEvents[0].Body, "name=dummy-fail") {
		t.Fatalf("checks-blocked event body should name dummy-fail; got %q", blockedEvents[0].Body)
	}
}

// TestSubmitWithPreMergeChecks_BypassAllowsMerge covers AC #5: the same
// fixture + a checks_bypass annotation skips the named check, submit runs
// normally, and a checks-bypass event with the documented reason is recorded.
func TestSubmitWithPreMergeChecks_BypassAllowsMerge(t *testing.T) {
	r := newPreMergeRepo(t)
	r.writeDummyFailCheck()
	b := r.makeBead("pmc-bypass-001", map[string]any{
		"checks_bypass": []any{
			map[string]any{
				"name":   "dummy-fail",
				"reason": "fixture: dummy-fail is always-block by design; bypassed in this test scenario",
				"bead":   "pmc-bypass-001",
			},
		},
	})
	resultSHA := r.commitNoOp()

	runID := time.Now().UTC().Format("20060102T150405") + "-pmcbyp1"
	res := &ExecuteBeadResult{
		BeadID:       b.ID,
		BaseRev:      r.baseSHA,
		ResultRev:    resultSHA,
		ExecutionDir: filepath.Join(".ddx", "executions", runID),
	}

	submitCalled := false
	submit := func(req LandRequest) (*LandResult, error) {
		submitCalled = true
		if req.BeadID != b.ID {
			t.Fatalf("submit got unexpected bead %q; want %q", req.BeadID, b.ID)
		}
		return &LandResult{Status: "landed", NewTip: req.ResultRev}, nil
	}

	land, outcome, err := SubmitWithPreMergeChecks(
		context.Background(), r.dir, b, res,
		submit, r.store, "ddx-test-actor", "test-source", nil,
	)
	if err != nil {
		t.Fatalf("SubmitWithPreMergeChecks: %v", err)
	}
	if !submitCalled {
		t.Fatalf("submit MUST be called when the only blocking check is bypassed (AC #5)")
	}
	if outcome == nil {
		t.Fatalf("outcome required, got nil")
	}
	if outcome.Blocked {
		t.Fatalf("outcome must not be Blocked when only check is bypassed; got %+v", outcome)
	}
	if len(outcome.Bypassed) != 1 || outcome.Bypassed[0].Name != "dummy-fail" {
		t.Fatalf("Bypassed slice must record dummy-fail; got %+v", outcome.Bypassed)
	}
	if land == nil || land.Status != "landed" {
		t.Fatalf("LandResult must reflect the underlying submit result; got %+v", land)
	}

	// Bypass event recorded with the operator-supplied reason.
	evs := r.readEvents(b.ID)
	var bypassEvents []bead.BeadEvent
	for _, e := range evs {
		if e.Kind == "checks-bypass" {
			bypassEvents = append(bypassEvents, e)
		}
	}
	if len(bypassEvents) != 1 {
		t.Fatalf("expected exactly one checks-bypass event; got %d (events=%+v)", len(bypassEvents), evs)
	}
	if !strings.Contains(bypassEvents[0].Body, "reason=fixture: dummy-fail is always-block") {
		t.Fatalf("checks-bypass event body must echo the operator-supplied reason; got %q", bypassEvents[0].Body)
	}
}

// TestSubmitWithPreMergeChecks_BypassMissingReasonRejected covers the
// "missing reason → bypass rejected loudly" half of AC #3: when the
// annotation has an empty reason, the gate must NOT honour the bypass and
// instead preserve the worker's result (treating the misconfigured bypass as
// a hard preserve, not a silent merge).
func TestSubmitWithPreMergeChecks_BypassMissingReasonRejected(t *testing.T) {
	r := newPreMergeRepo(t)
	r.writeDummyFailCheck()
	b := r.makeBead("pmc-badbypass-001", map[string]any{
		"checks_bypass": []any{
			map[string]any{
				"name":   "dummy-fail",
				"reason": "   ", // whitespace-only counts as missing
			},
		},
	})
	resultSHA := r.commitNoOp()

	runID := time.Now().UTC().Format("20060102T150405") + "-pmcbad1"
	res := &ExecuteBeadResult{
		BeadID:       b.ID,
		BaseRev:      r.baseSHA,
		ResultRev:    resultSHA,
		ExecutionDir: filepath.Join(".ddx", "executions", runID),
	}

	submitCalled := false
	submit := func(req LandRequest) (*LandResult, error) {
		submitCalled = true
		return &LandResult{Status: "landed"}, nil
	}

	land, _, err := SubmitWithPreMergeChecks(
		context.Background(), r.dir, b, res,
		submit, r.store, "ddx-test-actor", "test-source", nil,
	)
	if err != nil {
		t.Fatalf("SubmitWithPreMergeChecks: %v", err)
	}
	if submitCalled {
		t.Fatalf("submit MUST NOT be called when bypass annotation is invalid (AC #3)")
	}
	if land == nil || land.Status != "preserved" {
		t.Fatalf("LandResult must be preserved when bypass is rejected; got %+v", land)
	}
	if !strings.Contains(land.Reason, "checks_bypass") {
		t.Fatalf("preserve reason must mention the rejected checks_bypass; got %q", land.Reason)
	}
}

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
