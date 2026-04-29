package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TriageClassification values returned by the complexity evaluator.
const (
	TriageClassificationAtomic       = "atomic"
	TriageClassificationDecomposable = "decomposable"
	TriageClassificationAmbiguous    = "ambiguous"
)

// DefaultMaxDecompositionDepth is the default recursion cap for triage gate
// splitting. Configurable via agent.triage.max_decomposition_depth in
// .ddx/config.yaml.
const DefaultMaxDecompositionDepth = 3

// TriageSkipLabel on a bead bypasses the complexity gate without classifying.
const TriageSkipLabel = "triage:skip"

// TriageDepthKey is the bead Extra field key that tracks decomposition depth.
// Children filed by the gate inherit depth+1 from their parent.
const TriageDepthKey = "triage.depth"

// ChildBeadSpec describes one child bead produced by the splitter prompt.
type ChildBeadSpec struct {
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Acceptance   string   `json:"acceptance,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	SpecID       string   `json:"spec_id,omitempty"`
	InScopeFiles []string `json:"in_scope_files,omitempty"`
}

// TriageGate is called for each candidate bead between RoutePreflight and Claim.
// Returns (true, nil) when the bead is atomic and should proceed to Claim.
// Returns (false, nil) when the gate handled the bead (decomposed or surfaced
// for human triage). Returns (false, err) on a gate error — the loop logs the
// error and skips the bead without claiming.
type TriageGate func(ctx context.Context, candidate bead.Bead) (shouldClaim bool, err error)

// ComplexityClassifier classifies a bead as atomic, decomposable, or ambiguous.
// Returns the classification string, a confidence in [0,1], and a human-readable
// reasoning string.
type ComplexityClassifier func(ctx context.Context, b bead.Bead) (classification string, confidence float64, reasoning string, err error)

// BeadSplitter decomposes a decomposable bead into child specs.
type BeadSplitter func(ctx context.Context, b bead.Bead) (children []ChildBeadSpec, rationale string, err error)

// TriageStore is the store interface required by the triage gate for child
// filing and parent mutation. *bead.Store satisfies this interface.
type TriageStore interface {
	Create(b *bead.Bead) error
	Update(id string, fn func(*bead.Bead)) error
	AppendEvent(id string, ev bead.BeadEvent) error
}

// NewComplexityGate constructs a TriageGate that evaluates each candidate bead
// for decomposition before Claim. Behaviour by classification:
//   - atomic:       returns shouldClaim=true, caller proceeds to Claim.
//   - decomposable: files child beads, sets parent status=blocked, returns
//     shouldClaim=false.
//   - ambiguous:    sets execution-eligible=false, appends triage-ambiguous
//     event, returns shouldClaim=false.
//
// At depth cap: emits kind:triage-overflow, sets status=blocked with
// label=needs-human-decomposition, returns shouldClaim=false without calling
// the classifier.
//
// A nil gate (not using NewComplexityGate) causes the loop to skip triage and
// emit a one-time boot warning per Run() invocation.
func NewComplexityGate(classifier ComplexityClassifier, splitter BeadSplitter, store TriageStore, maxDepth int, now func() time.Time) TriageGate {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDecompositionDepth
	}
	if now == nil {
		now = time.Now
	}
	return func(ctx context.Context, candidate bead.Bead) (bool, error) {
		// triage:skip bypasses the gate entirely — bead proceeds to Claim.
		if HasBeadLabel(candidate.Labels, TriageSkipLabel) {
			return true, nil
		}

		depth := triageDepth(candidate)

		// At depth cap: park for human intervention, never dispatch.
		if depth >= maxDepth {
			body, _ := json.Marshal(map[string]any{"depth": depth, "max": maxDepth})
			_ = store.AppendEvent(candidate.ID, bead.BeadEvent{
				Kind:      "triage-overflow",
				Summary:   fmt.Sprintf("decomposition depth cap reached (%d)", maxDepth),
				Body:      string(body),
				Actor:     "ddx",
				Source:    "ddx triage gate",
				CreatedAt: now().UTC(),
			})
			_ = store.Update(candidate.ID, func(b *bead.Bead) {
				b.Status = bead.StatusBlocked
				if b.Extra == nil {
					b.Extra = make(map[string]any)
				}
				b.Extra["triage_block_reason"] = "needs-human-decomposition"
				if !HasBeadLabel(b.Labels, "needs-human-decomposition") {
					b.Labels = append(b.Labels, "needs-human-decomposition")
				}
			})
			return false, nil
		}

		classification, confidence, reasoning, err := classifier(ctx, candidate)
		if err != nil {
			return false, fmt.Errorf("triage classifier: %w", err)
		}

		switch classification {
		case TriageClassificationAtomic:
			return true, nil

		case TriageClassificationAmbiguous:
			body, _ := json.Marshal(map[string]any{
				"confidence": confidence,
				"reasoning":  reasoning,
			})
			_ = store.AppendEvent(candidate.ID, bead.BeadEvent{
				Kind:      "triage-ambiguous",
				Summary:   "triage: ambiguous — needs human review",
				Body:      string(body),
				Actor:     "ddx",
				Source:    "ddx triage gate",
				CreatedAt: now().UTC(),
			})
			_ = store.Update(candidate.ID, func(b *bead.Bead) {
				if b.Extra == nil {
					b.Extra = make(map[string]any)
				}
				b.Extra["execution-eligible"] = false
				b.Extra["triage_block_reason"] = "ambiguous"
				if !HasBeadLabel(b.Labels, "triage:needs-human") {
					b.Labels = append(b.Labels, "triage:needs-human")
				}
			})
			return false, nil

		case TriageClassificationDecomposable:
			children, rationale, err := splitter(ctx, candidate)
			if err != nil {
				return false, fmt.Errorf("triage splitter: %w", err)
			}
			childIDs, err := fileChildren(store, candidate, children, depth, now)
			if err != nil {
				return false, err
			}
			blockParentDecomposed(store, candidate.ID, childIDs, rationale, now)
			return false, nil
		}

		// Unknown classification falls through to Claim.
		return true, nil
	}
}

// triageDepth reads the current decomposition depth from a bead's Extra field.
// JSON numbers unmarshal as float64; the switch handles all common numeric types.
func triageDepth(b bead.Bead) int {
	if b.Extra == nil {
		return 0
	}
	switch v := b.Extra[TriageDepthKey].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return 0
}

// fileChildren creates child beads in the store and returns their IDs.
func fileChildren(store TriageStore, parent bead.Bead, specs []ChildBeadSpec, parentDepth int, now func() time.Time) ([]string, error) {
	childIDs := make([]string, 0, len(specs))
	for _, spec := range specs {
		labels := make([]string, len(spec.Labels))
		copy(labels, spec.Labels)

		extra := map[string]any{TriageDepthKey: parentDepth + 1}

		// Inherit spec-id from parent.
		if parent.Extra != nil {
			if specID, ok := parent.Extra["spec-id"]; ok && specID != "" {
				extra["spec-id"] = specID
			}
		}
		if spec.SpecID != "" {
			extra["spec-id"] = spec.SpecID
		}

		child := &bead.Bead{
			Title:       spec.Title,
			Description: spec.Description,
			Acceptance:  spec.Acceptance,
			Labels:      labels,
			Parent:      parent.ID,
			Extra:       extra,
		}
		if err := store.Create(child); err != nil {
			return nil, fmt.Errorf("triage: file child bead %q: %w", spec.Title, err)
		}
		childIDs = append(childIDs, child.ID)
	}
	return childIDs, nil
}

// blockParentDecomposed marks the parent as blocked-by-children: sets
// status=blocked, adds deps on children (so they must close first), and
// appends a kind:triage-decomposed event listing the child IDs.
func blockParentDecomposed(store TriageStore, parentID string, childIDs []string, rationale string, now func() time.Time) {
	body, _ := json.Marshal(map[string]any{
		"child_ids": childIDs,
		"rationale": rationale,
	})
	_ = store.AppendEvent(parentID, bead.BeadEvent{
		Kind:      "triage-decomposed",
		Summary:   fmt.Sprintf("decomposed into %d children: %v", len(childIDs), childIDs),
		Body:      string(body),
		Actor:     "ddx",
		Source:    "ddx triage gate",
		CreatedAt: now().UTC(),
	})
	_ = store.Update(parentID, func(b *bead.Bead) {
		b.Status = bead.StatusBlocked
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["triage_block_reason"] = "blocked-by-children"
		b.Extra["triage_child_ids"] = childIDs
		if !HasBeadLabel(b.Labels, "triage:decomposed") {
			b.Labels = append(b.Labels, "triage:decomposed")
		}
		// Dependency edges ensure ReadyExecution excludes the parent until
		// all children close (when/if the parent is later re-opened).
		for _, childID := range childIDs {
			b.AddDep(childID, "blocks")
		}
	})
}

// ACCoverageRate computes the fraction of unique AC tokens from the parent that
// appear in the combined AC text of the child specs. This is the string-overlap
// metric used by the TriagePrompts tests to assert >= 90% coverage after a split.
// Tokens shorter than 3 characters are ignored as stop-words.
func ACCoverageRate(parentAC string, children []ChildBeadSpec) float64 {
	parentTokens := tokenizeAC(parentAC)
	if len(parentTokens) == 0 {
		return 1.0
	}

	// Deduplicate parent tokens.
	parentSet := make(map[string]bool, len(parentTokens))
	for _, t := range parentTokens {
		parentSet[t] = true
	}

	var childBuilder strings.Builder
	for _, c := range children {
		childBuilder.WriteString(" ")
		childBuilder.WriteString(c.Acceptance)
	}
	childTokens := tokenizeAC(childBuilder.String())
	childSet := make(map[string]bool, len(childTokens))
	for _, t := range childTokens {
		childSet[t] = true
	}

	covered := 0
	for t := range parentSet {
		if childSet[t] {
			covered++
		}
	}
	return float64(covered) / float64(len(parentSet))
}

// tokenizeAC normalises and splits acceptance-criteria text into lowercase
// alphanumeric tokens of length >= 3 for the AC-coverage overlap metric.
func tokenizeAC(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			if cur.Len() >= 3 {
				tokens = append(tokens, cur.String())
			}
			cur.Reset()
		}
	}
	if cur.Len() >= 3 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// RuleBasedClassifier is a heuristic ComplexityClassifier used by the
// TriagePrompts tests to validate the eval corpus without requiring a live LLM.
// It matches keywords derived from the complexity-eval prompt criteria.
func RuleBasedClassifier(_ context.Context, b bead.Bead) (string, float64, string, error) {
	text := strings.ToLower(b.Title + " " + b.Description + " " + b.Acceptance)

	decomposableKWs := []string{
		"all crud", "complete suite", "multiple ", "all endpoints", "all operations",
		"batch ", "matrix ", "initial tranche", "full system", "migrate all",
		"monitoring, alerting", "phase 1", "phase 2", "part 1", "part 2",
		"and dashboard", "scaffolding", "full migration", "epic", "all services",
		"step 1", "step 2", "auth system", "infrastructure", "suite of",
	}
	atomicKWs := []string{
		"fix nil", "fix bug", "add flag", "add --", "bump ", "update default",
		"rename ", "single file", "one endpoint", "one method",
	}

	decomScore := 0
	for _, kw := range decomposableKWs {
		if strings.Contains(text, kw) {
			decomScore++
		}
	}
	atomicScore := 0
	for _, kw := range atomicKWs {
		if strings.Contains(text, kw) {
			atomicScore++
		}
	}

	if decomScore >= 2 && decomScore > atomicScore {
		return TriageClassificationDecomposable, 0.82, "multiple deliverables detected", nil
	}
	return TriageClassificationAtomic, 0.87, "focused single-deliverable task", nil
}
