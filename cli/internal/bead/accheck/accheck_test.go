package accheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ----- classifier tests -----

func TestParseAcceptance_NumberedItems(t *testing.T) {
	in := `1. First AC line.
2. Second AC line.
3. Third AC line.`
	items := ParseAcceptance(in)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, it := range items {
		if it.AC != i+1 {
			t.Errorf("item %d: expected AC=%d, got %d", i, i+1, it.AC)
		}
	}
}

func TestClassify_TestNameAC(t *testing.T) {
	k, name := classify("TestFooBar passes against fixture X")
	if k != KindTestName {
		t.Errorf("kind: want %s, got %s", KindTestName, k)
	}
	if name != "TestFooBar" {
		t.Errorf("name: want TestFooBar, got %q", name)
	}
}

func TestClassify_SymbolAC_Backtick(t *testing.T) {
	k, name := classify("`RemovedExecutionDirs` counter is incremented per delete")
	if k != KindSymbol {
		t.Errorf("kind: want %s, got %s", KindSymbol, k)
	}
	if name != "RemovedExecutionDirs" {
		t.Errorf("name: want RemovedExecutionDirs, got %q", name)
	}
}

func TestClassify_NegativeAC(t *testing.T) {
	k, name := classify("Land() no longer calls `git.Fetch` directly")
	if k != KindNegative {
		t.Errorf("kind: want %s, got %s", KindNegative, k)
	}
	if name != "git.Fetch" {
		t.Errorf("name: want git.Fetch, got %q", name)
	}
}

func TestClassify_BuildGate(t *testing.T) {
	k, _ := classify("lefthook run pre-commit passes")
	if k != KindBuildGate {
		t.Errorf("kind: want %s, got %s", KindBuildGate, k)
	}
}

func TestClassify_BuildGate_GoRun(t *testing.T) {
	k, _ := classify("go run golang.org/x/tools/cmd/deadcode ./... | rg unused")
	if k != KindBuildGate {
		t.Errorf("kind: want %s, got %s", KindBuildGate, k)
	}
}

func TestClassify_BuildGate_BashScripts(t *testing.T) {
	k, _ := classify("bash scripts/check-formatting.sh exits with 0")
	if k != KindBuildGate {
		t.Errorf("kind: want %s, got %s", KindBuildGate, k)
	}
}

func TestClassify_BuildGate_BunRun(t *testing.T) {
	k, _ := classify("bun run test:e2e passes")
	if k != KindBuildGate {
		t.Errorf("kind: want %s, got %s", KindBuildGate, k)
	}
}

func TestClassify_FilePath(t *testing.T) {
	cases := []string{
		"cli/cmd/work.go:133 contains the entry point",
		"Modify cli/internal/agent/preclaim_ac_quality.go:99 to add new pattern",
		"File cli/internal/bead/accheck/accheck.go:180 has the regex",
		"See docs/helix/06-iterate/bead-authoring-template.md:42 for details",
		"Add code to cli/internal/server/frontend/src/App.svelte:15",
		"Update config.yaml:25 to add new setting",
		"Check test result in tests/e2e.ts:100",
	}
	for _, text := range cases {
		k, _ := classify(text)
		if k != KindFilePath {
			t.Errorf("classify(%q): want %s, got %s", text, KindFilePath, k)
		}
	}
}

func TestClassify_Mechanical_Rename(t *testing.T) {
	k, _ := classify("rename module path from x to y")
	if k != KindMechanical {
		t.Errorf("kind: want %s, got %s", KindMechanical, k)
	}
}

// AC #1: quoted command followed by an outcome verb is classified as
// KindCommand (verifiable), not KindProse.
func TestClassify_CommandAC_QuotedReturns(t *testing.T) {
	cases := []string{
		"'ls scripts/benchmark/bench-sets/*.yaml | wc -l' returns 7.",
		`'python -c "import yaml; yaml.safe_load(open(\"x.yaml\"))"' exits 0.`,
		"`./scripts/check.sh` passes.",
		"\"jq '.foo' bar.json\" outputs the expected list.",
	}
	for _, text := range cases {
		k, _ := classify(text)
		if k != KindCommand {
			t.Errorf("classify(%q): want %s, got %s", text, KindCommand, k)
		}
	}
}

// AC #2: unquoted command-shaped AC followed by an outcome verb is also
// classified as KindCommand.
func TestClassify_CommandAC_UnquotedReturns(t *testing.T) {
	cases := []string{
		"ls foo | wc -l returns 7",
		"python -c 'import yaml' exits 0",
		"jq .foo bar.json outputs the right value",
	}
	for _, text := range cases {
		k, _ := classify(text)
		if k != KindCommand {
			t.Errorf("classify(%q): want %s, got %s", text, KindCommand, k)
		}
	}
}

// AC #3: prose-only ACs (no command at start, no outcome verb) still
// classify as KindProse.
func TestClassify_CommandAC_DoesNotMatchProse(t *testing.T) {
	cases := []string{
		"Each bench-set's task list is a subset of the current sweep entries.",
		"Improve logging clarity",
		"more readable code",
	}
	for _, text := range cases {
		k, _ := classify(text)
		if k != KindProse {
			t.Errorf("classify(%q): want %s, got %s", text, KindProse, k)
		}
	}
}

// Evaluator must report needs_judgment for a command AC: ac-check itself
// does not execute commands; the orchestrator/operator runs them and the
// reviewer ratifies.
func TestACCheck_CommandAC_NeedsJudgment(t *testing.T) {
	item := Item{AC: 1, Text: "'ls foo | wc -l' returns 7", Kind: KindCommand}
	e := evaluateOne(item, Context{})
	if e.Kind != KindCommand {
		t.Errorf("entry kind: want %s, got %s", KindCommand, e.Kind)
	}
	if e.Result != ResultNeedsJudgment {
		t.Fatalf("command AC must yield needs_judgment; got %s (%s)", e.Result, e.Evidence)
	}
}

func TestClassify_Prose_Default(t *testing.T) {
	k, _ := classify("improve readability of the error message")
	if k != KindProse {
		t.Errorf("kind: want %s, got %s", KindProse, k)
	}
}

// ----- evaluator tests (using injected hooks) -----

func ctxWithHooks(tb testing.TB, runTest func(name string, packages []string) (bool, string, error), grep func(symbol string) (int, string, error), diff func(symbol string) (int, error)) Context {
	tb.Helper()
	return Context{
		WorkingDir: ".",
		RunTest:    runTest,
		GitGrep:    grep,
		DiffHits:   diff,
	}
}

func TestACCheck_TestNameAC_Passes(t *testing.T) {
	item := Item{AC: 1, Text: "TestFoo passes", Kind: KindTestName, Name: "TestFoo"}
	ctx := ctxWithHooks(t,
		func(name string, packages []string) (bool, string, error) {
			if name != "TestFoo" {
				return false, "wrong name", nil
			}
			return true, "ok 0.04s", nil
		}, nil, nil)
	e := evaluateOne(item, ctx)
	if e.Result != ResultPass {
		t.Fatalf("expected pass, got %s (%s)", e.Result, e.Evidence)
	}
}

func TestACCheck_TestNameAC_Renamed_WithAlias(t *testing.T) {
	item := Item{AC: 1, Text: "TestFooBar passes", Kind: KindTestName, Name: "TestFooBar"}
	ctx := ctxWithHooks(t,
		func(name string, packages []string) (bool, string, error) {
			// Should be invoked with the alias, not the original.
			if name != "TestFoo/bar" {
				return false, "expected alias invocation", nil
			}
			return true, "ok via alias", nil
		}, nil, nil)
	ctx.Aliases = map[int]string{1: "TestFoo/bar"}
	entries := Evaluate([]Item{item}, ctx)
	if entries[0].Result != ResultPass {
		t.Fatalf("expected pass with alias, got %s (%s)", entries[0].Result, entries[0].Evidence)
	}
}

func TestACCheck_TestNameAC_Missing_Fails(t *testing.T) {
	item := Item{AC: 1, Text: "TestMissing passes", Kind: KindTestName, Name: "TestMissing"}
	ctx := ctxWithHooks(t,
		func(name string, packages []string) (bool, string, error) {
			return false, "[no tests to run]", nil
		}, nil, nil)
	e := evaluateOne(item, ctx)
	if e.Result != ResultFail {
		t.Fatalf("expected fail, got %s", e.Result)
	}
}

func TestACCheck_NegativeAC_FlagsResidue(t *testing.T) {
	item := Item{AC: 1, Text: "Land() no longer calls `git.Fetch`", Kind: KindNegative, Name: "git.Fetch"}
	ctx := ctxWithHooks(t, nil,
		func(symbol string) (int, string, error) {
			return 3, "execute_bead_land.go:123", nil
		}, nil)
	e := evaluateOne(item, ctx)
	if e.Result != ResultFail {
		t.Fatalf("expected fail (residue present), got %s", e.Result)
	}
}

func TestACCheck_NegativeAC_AbsenceConfirmed_Passes(t *testing.T) {
	item := Item{AC: 1, Text: "Land() no longer calls `git.Fetch`", Kind: KindNegative, Name: "git.Fetch"}
	ctx := ctxWithHooks(t, nil,
		func(symbol string) (int, string, error) { return 0, "", nil }, nil)
	e := evaluateOne(item, ctx)
	if e.Result != ResultPass {
		t.Fatalf("expected pass (absence confirmed), got %s", e.Result)
	}
}

func TestACCheck_SymbolAC_PresentInDiff_Passes(t *testing.T) {
	item := Item{AC: 1, Text: "Add `RemovedDirs` counter", Kind: KindSymbol, Name: "RemovedDirs"}
	ctx := ctxWithHooks(t, nil,
		func(symbol string) (int, string, error) { return 5, "cleanup.go:88", nil },
		func(symbol string) (int, error) { return 2, nil })
	e := evaluateOne(item, ctx)
	if e.Result != ResultPass {
		t.Fatalf("expected pass, got %s (%s)", e.Result, e.Evidence)
	}
}

func TestACCheck_SymbolAC_NotInDiff_Fails(t *testing.T) {
	item := Item{AC: 1, Text: "Add `RemovedDirs` counter", Kind: KindSymbol, Name: "RemovedDirs"}
	// Symbol exists in tree but not in diff = false claim.
	ctx := ctxWithHooks(t, nil,
		func(symbol string) (int, string, error) { return 5, "cleanup.go:88 (pre-existing)", nil },
		func(symbol string) (int, error) { return 0, nil })
	e := evaluateOne(item, ctx)
	if e.Result != ResultFail {
		t.Fatalf("expected fail (not in diff), got %s (%s)", e.Result, e.Evidence)
	}
}

func TestACCheck_ProseAC_NeedsJudgment_NeverAutoFail(t *testing.T) {
	item := Item{AC: 1, Text: "improve readability of the error message", Kind: KindProse}
	e := evaluateOne(item, Context{})
	if e.Result != ResultNeedsJudgment {
		t.Fatalf("expected needs_judgment, got %s", e.Result)
	}
}

func TestACCheck_ProseAC_NeedsJudgment_NeverAutoPass(t *testing.T) {
	// Same as above, separate name to make the contract explicit (the result
	// must NEVER be ResultPass for a prose AC).
	item := Item{AC: 1, Text: "logs become clearer", Kind: KindProse}
	e := evaluateOne(item, Context{})
	if e.Result == ResultPass {
		t.Fatalf("prose AC must never auto-pass; got %s", e.Result)
	}
}

func TestACCheck_BuildGateAC_NeedsJudgment(t *testing.T) {
	item := Item{AC: 1, Text: "lefthook run pre-commit passes", Kind: KindBuildGate}
	e := evaluateOne(item, Context{})
	if e.Result != ResultNeedsJudgment {
		t.Fatalf("expected needs_judgment for build gate, got %s", e.Result)
	}
}

func TestACCheck_MechanicalAC_NeedsJudgment(t *testing.T) {
	item := Item{AC: 1, Text: "rename module path from x to y", Kind: KindMechanical}
	e := evaluateOne(item, Context{})
	if e.Result != ResultNeedsJudgment {
		t.Fatalf("expected needs_judgment for mechanical AC, got %s", e.Result)
	}
}

func TestACCheck_MechanicalAC_RenameDetection(t *testing.T) {
	// Verify that a rename AC is classified as mechanical and that the
	// evaluator yields needs_judgment (no file-path resolver provided).
	// This mirrors the real rename-detection path: without a concrete path,
	// the reviewer adjudicates against git diff --name-status.
	k, _ := classify("rename execute_bead_land.go to bead_land.go")
	if k != KindMechanical {
		t.Errorf("classify: want %s, got %s", KindMechanical, k)
	}
	item := Item{AC: 2, Text: "rename execute_bead_land.go to bead_land.go", Kind: KindMechanical}
	e := evaluateOne(item, Context{})
	if e.Kind != KindMechanical {
		t.Errorf("entry kind: want %s, got %s", KindMechanical, e.Kind)
	}
	if e.Result != ResultNeedsJudgment {
		t.Fatalf("rename AC must yield needs_judgment; got %s (%s)", e.Result, e.Evidence)
	}
	if e.Evidence == "" {
		t.Error("expected non-empty evidence string for rename detection AC")
	}
}

// ----- output tests -----

func TestACCheck_JSONOutput_SchemaVersion1(t *testing.T) {
	out := Aggregate("ddx-test", "20260510T210000-abcdef00", []Entry{
		{AC: 1, Kind: KindSymbol, Result: ResultPass, Evidence: "ok"},
	})
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"schema_version": 1`) {
		t.Errorf("output missing schema_version=1; got: %s", string(b))
	}
}

func TestACCheck_Aggregate_Counts(t *testing.T) {
	out := Aggregate("ddx-x", "", []Entry{
		{AC: 1, Result: ResultPass},
		{AC: 2, Result: ResultFail},
		{AC: 3, Result: ResultNeedsJudgment},
		{AC: 4, Result: ResultPass},
		{AC: 5, Result: ResultError},
	})
	if out.Summary.Pass != 2 || out.Summary.Fail != 1 || out.Summary.NeedsJudgment != 1 || out.Summary.Error != 1 {
		t.Errorf("summary mismatch: %+v", out.Summary)
	}
}

func TestWriteHuman_RendersTable(t *testing.T) {
	out := Aggregate("ddx-x", "", []Entry{
		{AC: 1, Kind: KindSymbol, Result: ResultPass, Evidence: "ok"},
		{AC: 2, Kind: KindProse, Result: ResultNeedsJudgment, Evidence: "defer"},
	})
	var buf bytes.Buffer
	if err := WriteHuman(&buf, out); err != nil {
		t.Fatalf("WriteHuman: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "AC #1") || !strings.Contains(s, "AC #2") {
		t.Errorf("expected per-AC lines, got:\n%s", s)
	}
	if !strings.Contains(s, "summary:") {
		t.Errorf("expected summary line, got:\n%s", s)
	}
}

// ----- regression test for the fake-work pattern -----

// TestACCheck_RegressionFakeRetainDays replays the shape of the fake
// commits b7c18c7a1 / d73eaffc6 / 34b412254 (each claimed "implement
// pruneAgentLogs / pruneWorkerDirs / counters / 5 tests" but only changed
// a default value). The ac-check tool MUST report failure for the AC items
// that named missing symbols and tests.
func TestACCheck_RegressionFakeRetainDays(t *testing.T) {
	// Mimic the bead's acceptance text (abbreviated).
	acceptance := `1. cli/internal/agent/execution_cleanup.go has a ` + "`pruneOlderThan`" + ` helper.
2. ` + "`RemovedExecutionDirs`" + `, ` + "`RemovedAgentLogs`" + `, ` + "`RemovedWorkerDirs`" + ` counters are added.
3. Default RetainDays is 90 (not 7).
4. TestExecutionCleanup_DeletesOldAgentLogs passes.
5. lefthook run pre-commit passes.`
	items := ParseAcceptance(acceptance)
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}

	// Simulate the fake-work diff: only types.go default value changed.
	// No pruneOlderThan in the tree. No counters added. No test added.
	ctx := Context{
		RunTest: func(name string, _ []string) (bool, string, error) {
			// No matching test exists in the fake diff.
			return false, "[no tests to run]", nil
		},
		GitGrep: func(symbol string) (int, string, error) {
			// None of the claimed symbols exist post-image.
			return 0, "", nil
		},
		DiffHits: func(symbol string) (int, error) { return 0, nil },
	}

	entries := Evaluate(items, ctx)
	out := Aggregate("ddx-cccb4763", "test-replay", entries)

	// AC 1: pruneOlderThan symbol → fail (0 grep hits in post-image).
	if entries[0].Result != ResultFail {
		t.Errorf("AC #1 (pruneOlderThan): want fail, got %s — %s", entries[0].Result, entries[0].Evidence)
	}
	// AC 2: multiple counter symbols → fail.
	if entries[1].Result != ResultFail {
		t.Errorf("AC #2 (counters): want fail, got %s — %s", entries[1].Result, entries[1].Evidence)
	}
	// AC 3: default value (prose) → needs_judgment (not a symbol AC, not a fail).
	if entries[2].Result == ResultPass {
		t.Errorf("AC #3 (default 90): must not auto-pass; got %s", entries[2].Result)
	}
	// AC 4: TestExecutionCleanup_DeletesOldAgentLogs → fail (no matching test).
	if entries[3].Result != ResultFail {
		t.Errorf("AC #4 (test): want fail, got %s — %s", entries[3].Result, entries[3].Evidence)
	}
	// AC 5: build-gate → needs_judgment (orchestrator reports separately).
	if entries[4].Result != ResultNeedsJudgment {
		t.Errorf("AC #5 (lefthook): want needs_judgment, got %s", entries[4].Result)
	}

	// Overall: fake-work must NOT register as all-pass.
	if out.Summary.Fail < 2 {
		t.Errorf("fake-work AC-check should produce ≥ 2 fails; got %+v", out.Summary)
	}

	// Trace for humans reading test output.
	var buf bytes.Buffer
	_ = WriteHuman(&buf, out)
	t.Logf("regression replay:\n%s", buf.String())

	// Make sure JSON shape round-trips through encoding/json correctly so
	// downstream consumers can parse it.
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"schema_version"`) {
		t.Errorf("missing schema_version in output: %s", string(b))
	}
	_ = fmt.Sprintf("%v", b) // silence unused if formatter strips
}
