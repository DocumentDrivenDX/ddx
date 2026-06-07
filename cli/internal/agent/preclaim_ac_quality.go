package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/bead/accheck"
)

// DefaultACQualityMinScore is the default fraction of AC items that must be
// mechanically classifiable (non-prose) for the pre-claim quality gate to pass.
const DefaultACQualityMinScore = 0.5

// ACQualityItem holds the classification of one parsed AC line.
type ACQualityItem struct {
	AC         int    `json:"ac"`
	Text       string `json:"text"`
	Kind       string `json:"kind"`
	Verifiable bool   `json:"verifiable"`
}

// ACQualityResult is the verifiability assessment of a bead's acceptance text.
// Score = VerifiableCount / Total; PassesThreshold = Score >= Threshold.
type ACQualityResult struct {
	Score           float64         `json:"score"`
	VerifiableCount int             `json:"verifiable_count"`
	ProseCount      int             `json:"prose_count"`
	Total           int             `json:"total"`
	Items           []ACQualityItem `json:"items"`
	PassesThreshold bool            `json:"passes_threshold"`
	Threshold       float64         `json:"threshold"`
}

// BeadACQualityStore is the minimal store interface required by the AC quality gate.
type BeadACQualityStore interface {
	Get(ctx context.Context, id string) (*bead.Bead, error)
	Update(ctx context.Context, id string, mutate func(*bead.Bead)) error
	AppendEvent(id string, event bead.BeadEvent) error
}

// CheckACQuality parses acceptance text and computes a verifiability score.
// verifiable_count = items classified as test-name | symbol | negative |
// build-gate | mechanical. score = verifiable_count / total; 0 when total == 0.
func CheckACQuality(acceptance string, threshold float64) ACQualityResult {
	items := accheck.ParseAcceptance(acceptance)
	total := len(items)
	result := ACQualityResult{
		Threshold: threshold,
		Total:     total,
		Items:     make([]ACQualityItem, 0, total),
	}
	if total == 0 {
		// No ACs to evaluate: pass vacuously. The gate targets beads that have
		// acceptance criteria but those criteria are all prose; absence of ACs
		// is a separate quality signal handled by other checks.
		result.PassesThreshold = true
		return result
	}
	for _, item := range items {
		verifiable := item.Kind != accheck.KindProse
		result.Items = append(result.Items, ACQualityItem{
			AC:         item.AC,
			Text:       item.Text,
			Kind:       string(item.Kind),
			Verifiable: verifiable,
		})
		if verifiable {
			result.VerifiableCount++
		} else {
			result.ProseCount++
		}
	}
	result.Score = float64(result.VerifiableCount) / float64(total)
	result.PassesThreshold = result.Score >= threshold
	return result
}

// MarkBeadACQualityLow emits the low-quality evidence event and, when block is
// true, sets execution-eligible=false so the bead leaves ReadyExecution.
// Safe to call more than once (idempotent label add; events are append-only).
func MarkBeadACQualityLow(store BeadACQualityStore, beadID string, result ACQualityResult, block bool) error {
	if err := store.Update(context.Background(), beadID, func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		if block {
			b.Extra["execution-eligible"] = false
		}
		if !beadHasACQualityLabel(b.Labels) {
			b.Labels = append(b.Labels, "ac-quality:needs-refinement")
		}
	}); err != nil {
		return fmt.Errorf("ac-quality: update bead: %w", err)
	}
	body, _ := json.Marshal(result.Items)
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind: "ac-quality-low",
		Summary: fmt.Sprintf("score=%.2f threshold=%.2f verifiable=%d/%d",
			result.Score, result.Threshold, result.VerifiableCount, result.Total),
		Body:   string(body),
		Source: "preclaim-ac-quality",
	})
}

func beadHasACQualityLabel(labels []string) bool {
	for _, l := range labels {
		if l == "ac-quality:needs-refinement" {
			return true
		}
	}
	return false
}

// NewACQualityPreClaimGate returns a PreClaimIntakeHook that checks the bead's
// AC verifiability score before any execution attempt. In WARN-ONLY mode, low
// scores append warning evidence and still proceed to the inner hook. In
// BLOCK/factory mode, low scores mark the bead ineligible and return
// PreClaimIntakeOperatorRequired. When score >= threshold, inner is called (if
// non-nil) or PreClaimIntakeActionableAtomic is returned.
//
// This gate is deterministic: no LLM call is made. It should be chained
// before the LLM-based intake hook so that beads with low-quality ACs never
// burn drain budget.
func NewACQualityPreClaimGate(store BeadACQualityStore, mode string, threshold float64, inner PreClaimIntakeHook) PreClaimIntakeHook {
	return func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return PreClaimIntakeResult{}, err
			}
		}
		b, err := store.Get(ctx, beadID)
		if err != nil {
			// Fail-open: delegate to inner hook on store read failure.
			if inner != nil {
				return inner(ctx, beadID)
			}
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		}
		result := CheckACQuality(b.Acceptance, threshold)
		if !result.PassesThreshold {
			block := isReadinessBlockingMode(mode)
			_ = MarkBeadACQualityLow(store, beadID, result, block)
			if !block {
				if inner != nil {
					return inner(ctx, beadID)
				}
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			}
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeOperatorRequired,
				Reason:  "ac-quality:needs-refinement",
				Detail: fmt.Sprintf(
					"score %.2f below threshold %.2f (%d/%d verifiable ACs)",
					result.Score, threshold, result.VerifiableCount, result.Total),
			}, nil
		}
		if inner != nil {
			return inner(ctx, beadID)
		}
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
	}
}
