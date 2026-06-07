package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreClaimACQuality_AllMechanicalACs_Passes(t *testing.T) {
	acceptance := "1. `ParseAcceptance` function exists in accheck\n2. TestFooBar passes\n3. cd cli && go test ./internal/agent/... green\n"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.True(t, result.PassesThreshold)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.VerifiableCount)
	assert.Equal(t, 0, result.ProseCount)
	for _, item := range result.Items {
		assert.True(t, item.Verifiable, "AC #%d should be verifiable but kind=%s: %s", item.AC, item.Kind, item.Text)
	}
}

func TestPreClaimACQuality_AllProseACs_BlocksClaim(t *testing.T) {
	acceptance := "1. Improve logging clarity\n2. Make error messages cleaner\n3. Code is more readable\n"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.False(t, result.PassesThreshold)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 0, result.VerifiableCount)
	assert.Equal(t, 3, result.ProseCount)
	for _, item := range result.Items {
		assert.Equal(t, "prose", item.Kind, "AC #%d should be prose: %s", item.AC, item.Text)
		assert.False(t, item.Verifiable)
	}
}

func TestPreClaimACQuality_MixedACs_ScoreAboveThreshold_Passes(t *testing.T) {
	// 2 verifiable (test-name, symbol), 1 prose => score = 2/3 ≈ 0.667 >= 0.5
	acceptance := "1. TestFooBar passes\n2. `NewStore` function added\n3. improve clarity\n"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.True(t, result.PassesThreshold)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 2, result.VerifiableCount)
	assert.Equal(t, 1, result.ProseCount)
	assert.InDelta(t, 2.0/3.0, result.Score, 0.01)
}

func TestPreClaimACQuality_MixedACs_ScoreBelowThreshold_Routes(t *testing.T) {
	// 1 verifiable (test-name), 3 prose => score = 1/4 = 0.25 < 0.5
	acceptance := "1. TestFooBar passes\n2. improve clarity\n3. better error messages\n4. more readable code\n"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.False(t, result.PassesThreshold)
	assert.Equal(t, 4, result.Total)
	assert.Equal(t, 1, result.VerifiableCount)
	assert.Equal(t, 3, result.ProseCount)
	assert.InDelta(t, 0.25, result.Score, 0.01)
}

// Regression: replays the fizeau-e08d8228 acceptance text. Three of the four
// ACs are quoted shell commands followed by an outcome verb; the fourth is
// prose. Pre-fix this scored verifiable=0/4 (all classified as prose). The
// score must now be >= 0.50 so command-in-prose ACs no longer get
// needs-refinement labels at intake time.
func TestPreClaimACQuality_CommandInProseACs_ScoreAboveThreshold(t *testing.T) {
	acceptance := "1. 'ls scripts/benchmark/bench-sets/*.yaml | wc -l' returns 7.\n" +
		"2. 'python -c \"import yaml; [yaml.safe_load(open(f)) for f in __import__(\\\"glob\\\").glob(\\\"scripts/benchmark/bench-sets/*.yaml\\\")]\"' exits 0.\n" +
		"3. 'python -c \"import yaml; yaml.safe_load(open(\\\"scripts/benchmark/concurrency-groups.yaml\\\"))\"' exits 0.\n" +
		"4. Each bench-set's task list is a subset of the current terminalbench-2-1-sweep.yaml subsets entries.\n"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 4, result.Total)
	assert.GreaterOrEqual(t, result.Score, 0.50, "score below 0.50 means command-in-prose ACs were misclassified as prose")
	assert.True(t, result.PassesThreshold)
	assert.Equal(t, 3, result.VerifiableCount, "ACs 1-3 are runnable commands")
	assert.Equal(t, 1, result.ProseCount, "AC 4 is prose")
}

func TestPreClaimACQuality_TestFunctionNameCountsAsVerifiable(t *testing.T) {
	acceptance := "1. TestFoo_Bar_Baz passes"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.VerifiableCount)
	assert.True(t, result.PassesThreshold)
	assert.True(t, result.Items[0].Verifiable)
	assert.Equal(t, "test-name", result.Items[0].Kind)
}

func TestPreClaimACQuality_ConcreteGoRunCommandCountsAsVerifiable(t *testing.T) {
	acceptance := "1. go run golang.org/x/tools/cmd/deadcode ./... | rg foo should exit 0"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.VerifiableCount)
	assert.True(t, result.PassesThreshold)
	assert.True(t, result.Items[0].Verifiable)
	assert.Equal(t, "build-gate", result.Items[0].Kind)
}

func TestPreClaimACQuality_FilePathLineRefCountsAsVerifiable(t *testing.T) {
	acceptance := "1. cli/cmd/work.go:133 contains the entry point"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.VerifiableCount)
	assert.True(t, result.PassesThreshold)
	assert.True(t, result.Items[0].Verifiable)
	assert.Equal(t, "file-path", result.Items[0].Kind)
}

func TestPreClaimACQuality_PlainProseStillNotVerifiable(t *testing.T) {
	acceptance := "1. Operators should see better behavior in the queue."
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 0, result.VerifiableCount)
	assert.False(t, result.PassesThreshold)
	assert.False(t, result.Items[0].Verifiable)
	assert.Equal(t, "prose", result.Items[0].Kind)
}

func TestPreClaimACQuality_BashScriptsCommandCountsAsVerifiable(t *testing.T) {
	acceptance := "1. bash scripts/validate.sh runs without error"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.VerifiableCount)
	assert.True(t, result.PassesThreshold)
	assert.True(t, result.Items[0].Verifiable)
	assert.Equal(t, "build-gate", result.Items[0].Kind)
}

func TestPreClaimACQuality_BunRunCommandCountsAsVerifiable(t *testing.T) {
	acceptance := "1. bun run test:unit passes all tests"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.VerifiableCount)
	assert.True(t, result.PassesThreshold)
	assert.True(t, result.Items[0].Verifiable)
	assert.Equal(t, "build-gate", result.Items[0].Kind)
}

func TestPreClaimACQuality_MisclassifiedACsNowScore(t *testing.T) {
	// Regression test: replays a bead that previously scored low due to
	// misclassifying file-path and go-run ACs as prose.
	// AC#1: File path reference (was classified as prose, now file-path)
	// AC#2: go run command (was classified as prose, now build-gate)
	// AC#3-#5: test-name, symbol, build-gate (were verifiable before)
	// AC#6: plain prose (still prose)
	acceptance := "1. cli/internal/agent/preclaim_ac_quality.go:99 has the classifier emission\n" +
		"2. go run golang.org/x/tools/cmd/deadcode ./cli/... | rg preclaim logs no unused\n" +
		"3. TestPreClaimACQuality_FilePathRef passes\n" +
		"4. `ClassifyFilePath` function exists\n" +
		"5. cd cli && go test ./internal/agent/... green\n" +
		"6. Operators have better diagnostics\n"
	result := CheckACQuality(acceptance, DefaultACQualityMinScore)
	assert.Equal(t, 6, result.Total)
	assert.GreaterOrEqual(t, result.VerifiableCount, 4, "expected ≥4 verifiable ACs after fix (file-path, go-run, test, symbol, build-gate)")
	assert.True(t, result.PassesThreshold, "score %.2f should pass threshold %.2f", result.Score, result.Threshold)
	// Verify specific items:
	assert.True(t, result.Items[0].Verifiable, "AC#1 (file-path) should be verifiable")
	assert.True(t, result.Items[1].Verifiable, "AC#2 (go-run) should be verifiable")
	assert.True(t, result.Items[2].Verifiable, "AC#3 (test-name) should be verifiable")
	assert.True(t, result.Items[3].Verifiable, "AC#4 (symbol) should be verifiable")
	assert.True(t, result.Items[4].Verifiable, "AC#5 (build-gate) should be verifiable")
	assert.False(t, result.Items[5].Verifiable, "AC#6 (prose) should not be verifiable")
}

func TestPreClaimACQuality_LowQualityEmitsACQualityEvent(t *testing.T) {
	b := &bead.Bead{
		ID:         "ddx-test0001",
		Title:      "Test bead",
		Acceptance: "1. Improve clarity\n2. Better error messages\n",
	}
	store := &acQualityTestStore{b: b}

	result := CheckACQuality(b.Acceptance, DefaultACQualityMinScore)
	require.False(t, result.PassesThreshold, "expected all-prose ACs to fail threshold")

	err := MarkBeadACQualityLow(store, b.ID, result, true)
	require.NoError(t, err)

	require.Len(t, store.events, 1, "expected exactly one ac-quality-low event")
	ev := store.events[0]
	assert.Equal(t, "ac-quality-low", ev.Kind)
	assert.Contains(t, ev.Summary, "score=0.00")
	assert.Contains(t, ev.Summary, fmt.Sprintf("threshold=%.2f", DefaultACQualityMinScore))

	var items []ACQualityItem
	require.NoError(t, json.Unmarshal([]byte(ev.Body), &items))
	assert.Len(t, items, 2)

	// Bead must be marked ineligible and labeled.
	assert.Equal(t, false, store.b.Extra["execution-eligible"])
	assert.True(t, beadHasACQualityLabel(store.b.Labels))
}

// acQualityTestStore is a minimal in-memory store for AC quality tests.
type acQualityTestStore struct {
	b      *bead.Bead
	events []bead.BeadEvent
}

func (s *acQualityTestStore) Get(_ context.Context, id string) (*bead.Bead, error) {
	if s.b == nil || s.b.ID != id {
		return nil, fmt.Errorf("bead %s not found", id)
	}
	cp := *s.b
	return &cp, nil
}

func (s *acQualityTestStore) Update(ctx context.Context, id string, mutate func(*bead.Bead)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.b == nil || s.b.ID != id {
		return fmt.Errorf("bead %s not found", id)
	}
	b := *s.b
	if mutate == nil {
		return fmt.Errorf("missing mutate function")
	}
	mutate(&b)
	s.b = &b
	return nil
}

func (s *acQualityTestStore) AppendEvent(id string, event bead.BeadEvent) error {
	s.events = append(s.events, event)
	return nil
}
