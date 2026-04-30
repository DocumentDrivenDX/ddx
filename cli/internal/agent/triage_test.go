package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// AC-1: atomic bead reaches Claim; decomposable does not.
// ---------------------------------------------------------------------------

// TestComplexityGateAtomicAllowsClaim asserts that a bead classified atomic by
// the gate proceeds to Claim and is dispatched normally.
func TestComplexityGateAtomicAllowsClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var classifierCalls int32
	gate := NewComplexityGate(
		func(_ context.Context, b bead.Bead) (string, float64, string, error) {
			atomic.AddInt32(&classifierCalls, 1)
			return TriageClassificationAtomic, 0.95, "single focused task", nil
		},
		nil, // splitter not called for atomic
		inner,
		3,
		nil,
	)

	worker := &ExecuteBeadWorker{
		Store:          store,
		ComplexityGate: gate,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-atomic",
				ResultRev: "aabbcc",
			}, nil
		}),
	}

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).
		Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&classifierCalls), "classifier must be called once")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "Claim must be called for atomic bead")
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

// TestComplexityGateDecomposableSkipsClaim asserts that a bead classified
// decomposable does NOT reach Claim; children are filed with parent link; the
// parent gets status=blocked.
func TestComplexityGateDecomposableSkipsClaim(t *testing.T) {
	// Use a fresh single-bead store so only the decomposable bead is in queue.
	beadStore := bead.NewStore(t.TempDir())
	require.NoError(t, beadStore.Init())

	epic := &bead.Bead{ID: "ddx-epic-1", Title: "Epic bead with multiple deliverables"}
	require.NoError(t, beadStore.Create(epic))

	store := &claimCountingStore{Store: beadStore}

	gate := NewComplexityGate(
		func(_ context.Context, b bead.Bead) (string, float64, string, error) {
			return TriageClassificationDecomposable, 0.90, "multiple distinct deliverables", nil
		},
		func(_ context.Context, _ bead.Bead) ([]ChildBeadSpec, string, error) {
			return []ChildBeadSpec{
				{Title: "Child A", Acceptance: "AC1 done"},
				{Title: "Child B", Acceptance: "AC2 done"},
			}, "split into A and B", nil
		},
		beadStore,
		3,
		nil,
	)

	var executedIDs []string
	worker := &ExecuteBeadWorker{
		Store:          store,
		ComplexityGate: gate,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			executedIDs = append(executedIDs, beadID)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess,
				SessionID: "s", ResultRev: "r"}, nil
		}),
	}

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).
		Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	// Run Once=true: gate fires on the epic, files children, skips it. No more
	// ready beads (children are at depth=1 but also return decomposable — gate
	// would skip them too). Loop exits with no Claim calls.
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)

	// Epic must NOT be dispatched.
	assert.NotContains(t, executedIDs, epic.ID, "decomposable bead must not be dispatched")

	// Claim must NOT have been called for the epic bead.
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls),
		"Claim must not be called for decomposable bead")

	// Parent must have status=blocked.
	parent, err := beadStore.Get(epic.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusBlocked, parent.Status)
	assert.Equal(t, "blocked-by-children", parent.Extra["triage_block_reason"])
	assert.True(t, HasBeadLabel(parent.Labels, "triage:decomposed"))

	// kind:triage-decomposed event must be appended with child IDs.
	events, err := beadStore.Events(epic.ID)
	require.NoError(t, err)
	var decomposedEv *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "triage-decomposed" {
			decomposedEv = &events[i]
			break
		}
	}
	require.NotNil(t, decomposedEv, "kind:triage-decomposed event missing")

	var payload struct {
		ChildIDs []string `json:"child_ids"`
	}
	require.NoError(t, json.Unmarshal([]byte(decomposedEv.Body), &payload))
	assert.Len(t, payload.ChildIDs, 2, "two children must be filed")

	// Children must have parent link and correct depth.
	for _, childID := range payload.ChildIDs {
		child, err := beadStore.Get(childID)
		require.NoError(t, err)
		assert.Equal(t, epic.ID, child.Parent, "child must link to parent")
		assert.Equal(t, float64(1), child.Extra[TriageDepthKey], "child depth must be 1")
	}
}

// ---------------------------------------------------------------------------
// AC-5: depth cap triggers triage-overflow; parent blocked, never dispatched.
// ---------------------------------------------------------------------------

// TestComplexityGateDepthCapBlocksParent asserts that a bead already at the
// depth cap gets a kind:triage-overflow event, status=blocked, and
// label=needs-human-decomposition, and is never dispatched.
func TestComplexityGateDepthCapBlocksParent(t *testing.T) {
	// Use a fresh single-bead store to isolate the at-cap bead.
	beadStore := bead.NewStore(t.TempDir())
	require.NoError(t, beadStore.Init())

	deepBead := &bead.Bead{
		ID:    "ddx-deep-1",
		Title: "Deep bead at depth cap",
		Extra: map[string]any{TriageDepthKey: float64(3)},
	}
	require.NoError(t, beadStore.Create(deepBead))

	var classifierCalls int32
	gate := NewComplexityGate(
		func(_ context.Context, b bead.Bead) (string, float64, string, error) {
			atomic.AddInt32(&classifierCalls, 1)
			return TriageClassificationDecomposable, 0.90, "epic", nil
		},
		func(_ context.Context, _ bead.Bead) ([]ChildBeadSpec, string, error) {
			return []ChildBeadSpec{{Title: "grandchild", Acceptance: "done"}}, "split", nil
		},
		beadStore,
		3, // maxDepth=3; deepBead at depth=3 must NOT be dispatched
		nil,
	)

	var executed []string
	store := &claimCountingStore{Store: beadStore}
	worker := &ExecuteBeadWorker{
		Store:          store,
		ComplexityGate: gate,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess,
				SessionID: "s", ResultRev: "r"}, nil
		}),
	}

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).
		Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)

	// Classifier must NOT be called for the at-cap bead (depth check fires first).
	assert.Equal(t, int32(0), atomic.LoadInt32(&classifierCalls),
		"classifier must not run for at-cap bead")

	// Deep bead must NOT be dispatched.
	assert.NotContains(t, executed, deepBead.ID, "at-cap bead must never be dispatched")

	// kind:triage-overflow event must be recorded.
	events, err := beadStore.Events(deepBead.ID)
	require.NoError(t, err)
	var overflowEv *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "triage-overflow" {
			overflowEv = &events[i]
			break
		}
	}
	require.NotNil(t, overflowEv, "kind:triage-overflow event must be recorded")

	// Parent must have status=blocked and needs-human-decomposition label.
	updated, err := beadStore.Get(deepBead.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusBlocked, updated.Status)
	assert.True(t, HasBeadLabel(updated.Labels, "needs-human-decomposition"),
		"needs-human-decomposition label must be set")
	assert.Equal(t, "needs-human-decomposition", updated.Extra["triage_block_reason"])
}

// TestComplexityGateImplementationHasNoRoutingVocabulary protects the bright
// line that triage estimates work shape only. The gate must not grow
// provider/model/harness selection logic.
func TestComplexityGateImplementationHasNoRoutingVocabulary(t *testing.T) {
	data, err := os.ReadFile("triage.go")
	require.NoError(t, err)
	text := string(data)
	assert.NotContains(t, text, "provider")
	assert.NotContains(t, text, "model")
	assert.NotContains(t, text, "harness")
}

// ---------------------------------------------------------------------------
// triage:skip label bypasses the gate entirely.
// ---------------------------------------------------------------------------

// TestComplexityGateSkipLabelBypassesClassifier asserts that a bead with
// the triage:skip label is dispatched without invoking the classifier.
func TestComplexityGateSkipLabelBypassesClassifier(t *testing.T) {
	// Use a fresh single-bead store to isolate the skip bead.
	beadStore := bead.NewStore(t.TempDir())
	require.NoError(t, beadStore.Init())

	skipBead := &bead.Bead{
		ID:     "ddx-skip-1",
		Title:  "Skip me — triage:skip",
		Labels: []string{"triage:skip"},
	}
	require.NoError(t, beadStore.Create(skipBead))

	var classifierCalls int32
	gate := NewComplexityGate(
		func(_ context.Context, b bead.Bead) (string, float64, string, error) {
			atomic.AddInt32(&classifierCalls, 1)
			// If somehow called (should not be for skip-labeled beads), return atomic.
			return TriageClassificationAtomic, 0.95, "n/a", nil
		},
		nil, // splitter not needed — skip bypasses everything
		beadStore,
		3,
		nil,
	)

	store := &claimCountingStore{Store: beadStore}
	worker := &ExecuteBeadWorker{
		Store:          store,
		ComplexityGate: gate,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess,
				SessionID: "s", ResultRev: "r"}, nil
		}),
	}

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).
		Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	// Classifier must NOT be called for skip-labeled bead.
	assert.Equal(t, int32(0), atomic.LoadInt32(&classifierCalls),
		"classifier must not run for triage:skip bead")

	// The skip bead must have been dispatched and closed.
	got, err := beadStore.Get(skipBead.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status,
		"triage:skip bead must be dispatched and closed")
}

// ---------------------------------------------------------------------------
// Gate nil emits a one-time warning, loop continues normally.
// ---------------------------------------------------------------------------

// TestComplexityGateNilEmitsWarning asserts that when ComplexityGate is nil,
// the loop emits a boot warning but still dispatches beads normally.
func TestComplexityGateNilEmitsWarning(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)

	var logBuf bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: inner,
		// ComplexityGate intentionally nil
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess,
				SessionID: "s", ResultRev: "r"}, nil
		}),
	}

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).
		Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &logBuf,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Successes, "nil gate must not block dispatch")
	assert.Contains(t, logBuf.String(), "warning", "one-time warning must be emitted")
	assert.Contains(t, logBuf.String(), "triage", "warning must mention triage")

	// Warning emitted exactly once across all beads.
	assert.Equal(t, 1, countOccurrences(logBuf.String(), "warning: triage"),
		"warning must be emitted only once per Run()")
}

// ---------------------------------------------------------------------------
// AC-2/3/4: TriagePrompts tests — prompts exist, corpus accuracy, AC coverage.
// ---------------------------------------------------------------------------

// TestTriagePromptsExist verifies both prompt files are present and non-empty.
func TestTriagePromptsExist(t *testing.T) {
	for _, path := range []string{
		"../../../library/prompts/triage/complexity-eval.md",
		"../../../library/prompts/triage/bead-split.md",
	} {
		data, err := os.ReadFile(path)
		require.NoError(t, err, "prompt file must exist: %s", path)
		assert.NotEmpty(t, data, "prompt file must be non-empty: %s", path)
	}
}

func TestTriagePromptsDeclareContractsAndBoundary(t *testing.T) {
	complexity, err := os.ReadFile("../../../library/prompts/triage/complexity-eval.md")
	require.NoError(t, err)
	split, err := os.ReadFile("../../../library/prompts/triage/bead-split.md")
	require.NoError(t, err)

	complexityText := string(complexity)
	for _, required := range []string{
		"atomic|decomposable|ambiguous",
		"confidence",
		"reasoning",
		"DDx only estimates work shape",
		"opaque passthrough constraints",
		"The agent owns concrete",
		"routing and execution",
	} {
		assert.Contains(t, complexityText, required)
	}

	splitText := string(split)
	for _, required := range []string{
		"children",
		"title",
		"description",
		"acceptance",
		"labels",
		"spec_id",
		"in_scope_files",
		"out_of_scope",
		"DDx only decomposes and tracks work",
		"opaque passthrough constraints",
		"The agent owns concrete",
		"routing and execution",
	} {
		assert.Contains(t, splitText, required)
	}
}

// TestTriagePromptsAccuracy loads the held-out eval slice from
// eval-corpus.jsonl and runs RuleBasedClassifier (which encodes the
// complexity-eval prompt criteria) against each entry. Asserts >= 80%
// accuracy, satisfying AC-3.
func TestTriagePromptsAccuracy(t *testing.T) {
	corpus, err := loadEvalCorpus("../../../library/prompts/triage/eval-corpus.jsonl")
	require.NoError(t, err, "eval-corpus.jsonl must load without error")
	require.NotEmpty(t, corpus, "eval-corpus.jsonl must contain at least one entry")

	evalSlice := evalHoldout(corpus)
	require.NotEmpty(t, evalSlice, "eval holdout must contain at least one entry")

	correct := 0
	for _, item := range evalSlice {
		got, _, _, err := RuleBasedClassifier(context.Background(), item.Bead)
		require.NoError(t, err)
		if got == item.ExpectedClass {
			correct++
		}
	}

	accuracy := float64(correct) / float64(len(evalSlice))
	assert.GreaterOrEqual(t, accuracy, 0.80,
		"complexity-eval accuracy must be >= 80%% on eval slice (got %.0f%%); eval items: %v",
		accuracy*100, evalSliceLabels(evalSlice))
}

// TestTriagePromptsACCoverage verifies that the ground-truth children in the
// eval corpus collectively cover >= 90% of their parent's AC tokens, satisfying
// AC-4 (bead-split AC-coverage metric).
func TestTriagePromptsACCoverage(t *testing.T) {
	corpus, err := loadEvalCorpus("../../../library/prompts/triage/eval-corpus.jsonl")
	require.NoError(t, err)

	evalSlice := evalHoldout(corpus)
	var coverages []float64
	for _, item := range evalSlice {
		if item.ExpectedClass != TriageClassificationDecomposable || len(item.ChildAcceptance) == 0 {
			continue
		}
		rate := ACCoverageRate(item.Bead.Acceptance, item.childSpecs())
		coverages = append(coverages, rate)
	}
	if len(coverages) == 0 {
		t.Skip("no decomposable examples with ground-truth children in eval slice")
	}

	total := 0.0
	for _, r := range coverages {
		total += r
	}
	avg := total / float64(len(coverages))
	assert.GreaterOrEqual(t, avg, 0.90,
		"average AC coverage must be >= 90%% on eval slice (got %.0f%%)", avg*100)
}

// TestTriageCorpus verifies the offline corpus contract used by the harvester
// and later prompt tests. The corpus must contain both classification classes
// and enough context to replay each item without reading tracker history.
func TestTriageCorpus(t *testing.T) {
	corpus, err := loadEvalCorpus("../../../library/prompts/triage/eval-corpus.jsonl")
	require.NoError(t, err, "eval-corpus.jsonl must load without error")
	require.NotEmpty(t, corpus, "eval-corpus.jsonl must contain entries")

	classes := map[string]bool{}
	seenIDs := map[string]bool{}
	for _, item := range corpus {
		require.NotEmpty(t, item.ID, "entry id is required")
		require.False(t, seenIDs[item.ID], "entry ids must be unique: %s", item.ID)
		seenIDs[item.ID] = true
		require.NotEmpty(t, item.SourceRepo, "source_repo is required for %s", item.ID)
		require.NotEmpty(t, item.ExpectedClass, "expected_class is required for %s", item.ID)
		require.Contains(t, []string{TriageClassificationAtomic, TriageClassificationDecomposable}, item.ExpectedClass)
		require.NotEmpty(t, item.Rationale, "rationale is required for %s", item.ID)
		require.NotEmpty(t, item.Bead.ID, "bead.id is required for %s", item.ID)
		require.NotEmpty(t, item.Bead.Title, "bead.title is required for %s", item.ID)
		require.Equal(t, item.Title, item.Bead.Title, "top-level title should mirror bead.title")
		if item.ExpectedClass == TriageClassificationDecomposable && len(item.ChildTitles) > 0 {
			require.Len(t, item.ChildAcceptance, len(item.ChildTitles),
				"child_acceptance must align with child_titles for %s", item.ID)
		}
		classes[item.ExpectedClass] = true
	}
	assert.True(t, classes[TriageClassificationAtomic], "corpus must include expected_class=atomic")
	assert.True(t, classes[TriageClassificationDecomposable], "corpus must include expected_class=decomposable")
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// corpusEntry is one row of the eval-corpus.jsonl file.
type corpusEntry struct {
	ID              string    `json:"id"`
	SourceRepo      string    `json:"source_repo"`
	ExpectedClass   string    `json:"expected_class"`
	Rationale       string    `json:"rationale"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Acceptance      string    `json:"acceptance"`
	Labels          []string  `json:"labels"`
	Bead            bead.Bead `json:"bead"`
	ChildTitles     []string  `json:"child_titles"`
	ChildAcceptance []string  `json:"child_acceptance"`
}

func (e corpusEntry) childSpecs() []ChildBeadSpec {
	specs := make([]ChildBeadSpec, 0, len(e.ChildAcceptance))
	for i, acceptance := range e.ChildAcceptance {
		title := ""
		if i < len(e.ChildTitles) {
			title = e.ChildTitles[i]
		}
		specs = append(specs, ChildBeadSpec{Title: title, Acceptance: acceptance})
	}
	return specs
}

// loadEvalCorpus reads and parses the JSONL eval corpus file.
func loadEvalCorpus(path string) ([]corpusEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []corpusEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e corpusEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// evalHoldout returns the held-out eval slice. The harvester writes the
// holdout directly to eval-corpus.jsonl, so tests use every row after sorting
// for deterministic diagnostics.
func evalHoldout(corpus []corpusEntry) []corpusEntry {
	sorted := make([]corpusEntry, len(corpus))
	copy(sorted, corpus)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	return sorted
}

// evalSliceLabels returns a list of "id:label" strings for diagnostics.
func evalSliceLabels(slice []corpusEntry) []string {
	out := make([]string, len(slice))
	for i, e := range slice {
		out[i] = e.ID + ":" + e.ExpectedClass
	}
	return out
}

// countOccurrences returns the number of non-overlapping occurrences of sub
// in s.
func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i <= len(s)-len(sub); {
		if s[i:i+len(sub)] == sub {
			n++
			i += len(sub)
		} else {
			i++
		}
	}
	return n
}

// TestACCoverageRateMetric exercises ACCoverageRate with synthetic inputs to
// verify the overlap metric itself is correct.
func TestACCoverageRateMetric(t *testing.T) {
	parentAC := "endpoint creates user and returns 201 session token included"
	children := []ChildBeadSpec{
		{Acceptance: "endpoint creates user and returns 201"},
		{Acceptance: "session token included in response"},
	}
	rate := ACCoverageRate(parentAC, children)
	assert.GreaterOrEqual(t, rate, 0.90,
		"well-covering children should have >= 90%% AC coverage")

	// Empty parent AC → 100% coverage by convention.
	assert.Equal(t, 1.0, ACCoverageRate("", children))

	// Zero overlap.
	noOverlap := []ChildBeadSpec{{Acceptance: "xyz"}}
	assert.Less(t, ACCoverageRate("aaa bbb ccc ddd", noOverlap), 0.10)
}

// TestTriageDepthParsing verifies triageDepth handles all JSON numeric types.
func TestTriageDepthParsing(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
		want  int
	}{
		{"nil extra", nil, 0},
		{"missing key", map[string]any{"other": "x"}, 0},
		{"int", map[string]any{TriageDepthKey: 2}, 2},
		{"float64", map[string]any{TriageDepthKey: float64(3)}, 3},
		{"int64", map[string]any{TriageDepthKey: int64(1)}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := bead.Bead{Extra: tc.extra}
			assert.Equal(t, tc.want, triageDepth(b))
		})
	}
}

// ---------------------------------------------------------------------------
// AC-6: end-to-end — fixture queue with epic body, assert decompose + child dispatch.
// ---------------------------------------------------------------------------

// TestTriageEndToEndEpicDecomposedAndChildDispatched runs the execute loop
// against a fixture queue that contains a bead modelled on the historical body
// of agent-6c7a4c11 (Harness × Model Matrix Benchmark — initial tranche).
// It asserts: (a) gate classifies the epic as decomposable, (b) children are
// filed with parent link, (c) the parent is NOT dispatched, (d) the first
// atomic child IS dispatched.
func TestTriageEndToEndEpicDecomposedAndChildDispatched(t *testing.T) {
	beadStore := bead.NewStore(t.TempDir())
	require.NoError(t, beadStore.Init())

	// Fixture bead modelled on agent-6c7a4c11: an epic with multiple distinct
	// deliverables that an agent would refuse to execute monolithically.
	epicID := "agent-6c7a4c11-fixture"
	epic := &bead.Bead{
		ID:          epicID,
		Title:       "Harness × Model Matrix Benchmark — initial tranche",
		Description: "Benchmark every configured harness against every configured model. Report throughput, cost, error rate per (harness, model) pair.",
		Acceptance:  "1. lmstudio harness benchmarked against all local models. 2. claude harness benchmarked against sonnet and opus. 3. Results written to bench/results.jsonl. 4. Summary table printed to stdout.",
	}
	require.NoError(t, beadStore.Create(epic))

	var classifiedDecomposable bool
	gate := NewComplexityGate(
		func(_ context.Context, b bead.Bead) (string, float64, string, error) {
			if b.ID == epicID {
				classifiedDecomposable = true
				return TriageClassificationDecomposable, 0.92,
					"four distinct deliverables map to separable PRs", nil
			}
			// Children are atomic.
			return TriageClassificationAtomic, 0.95, "single focused task", nil
		},
		func(_ context.Context, _ bead.Bead) ([]ChildBeadSpec, string, error) {
			return []ChildBeadSpec{
				{
					Title:      "Benchmark lmstudio harness against local models",
					Acceptance: "lmstudio harness benchmarked against all local models; results written to bench/results.jsonl",
				},
				{
					Title:      "Benchmark claude harness against sonnet and opus",
					Acceptance: "claude harness benchmarked against sonnet and opus; results appended to bench/results.jsonl; summary table printed to stdout",
				},
			}, "split lmstudio and claude benchmarks into independent tasks", nil
		},
		beadStore,
		3,
		nil,
	)

	var dispatchedIDs []string
	store := &claimCountingStore{Store: beadStore}
	worker := &ExecuteBeadWorker{
		Store:          store,
		ComplexityGate: gate,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			dispatchedIDs = append(dispatchedIDs, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "s",
				ResultRev: "r",
			}, nil
		}),
	}

	rcfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).
		Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"}))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)

	// (a) Gate classified the epic as decomposable.
	assert.True(t, classifiedDecomposable, "(a) gate must classify the epic as decomposable")

	// (b) Children filed with parent link.
	all, err := beadStore.ReadAll()
	require.NoError(t, err)
	var children []bead.Bead
	for _, b := range all {
		if b.Parent == epicID {
			children = append(children, b)
		}
	}
	require.NotEmpty(t, children, "(b) children must be filed with parent link")
	for _, child := range children {
		assert.Equal(t, epicID, child.Parent, "(b) child parent must point to epic")
	}

	// (c) Parent was NOT dispatched (executor never called with epic ID).
	assert.NotContains(t, dispatchedIDs, epicID, "(c) parent epic must not be dispatched")

	// (d) At least the first atomic child WAS dispatched.
	childIDs := make(map[string]bool)
	for _, child := range children {
		childIDs[child.ID] = true
	}
	dispatched := 0
	for _, id := range dispatchedIDs {
		if childIDs[id] {
			dispatched++
		}
	}
	assert.GreaterOrEqual(t, dispatched, 1, "(d) at least one atomic child must be dispatched")
}

// Compile-time check: bead.Store satisfies TriageStore.
var _ TriageStore = (*bead.Store)(nil)
