package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// ReviewGroup reviews the same bead/result_rev with two reviewer slots.
// The helper shares one evidence bundle and prompt file across both slots and
// returns the structured reviewer results alongside the shared bundle
// metadata. It does not change execute-loop close/block behavior.
func (r *DefaultBeadReviewer) ReviewGroup(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error) {
	diff, err := r.gitShow(resultRev)
	if err != nil {
		return nil, fmt.Errorf("review-group: git show %s: %w", resultRev, err)
	}
	return r.reviewGroupWithDiff(ctx, beadID, resultRev, impl, diff, r.ProjectRoot, "")
}

func (r *DefaultBeadReviewer) reviewGroupWithDiff(ctx context.Context, beadID, resultRev string, impl ImplementerRouting, diff, reviewWorkDir, acCheckJSON string) (*ReviewGroupResult, error) {
	b, err := r.BeadStore.Get(ctx, beadID)
	if err != nil {
		return nil, fmt.Errorf("review-group: get bead %s: %w", beadID, err)
	}
	if reviewWorkDir == "" {
		reviewWorkDir = r.ProjectRoot
	}

	refs := ResolveGoverningRefs(r.ProjectRoot, b)
	iter := 1

	caps := r.Caps
	if caps.MaxPromptBytes == 0 {
		caps = evidence.DefaultCaps()
	}
	built := BuildReviewPromptBounded(b, iter, resultRev, diff, r.ProjectRoot, refs, BuildReviewPromptOptions{Caps: caps, ACCheckJSON: acCheckJSON})
	groupID := GenerateAttemptID()
	artifacts, err := createArtifactBundle(r.ProjectRoot, r.ProjectRoot, groupID)
	if err != nil {
		return nil, fmt.Errorf("review-group: create artifact bundle: %w", err)
	}
	if err := os.WriteFile(artifacts.PromptAbs, []byte(built.Prompt), 0o644); err != nil {
		return nil, fmt.Errorf("review-group: write prompt artifact: %w", err)
	}

	reviewHarness := r.Harness
	priorErrors := countPriorEscalationTriggers(r.EventReader, beadID, resultRev)
	reviewProfile := r.reviewerDispatchProfile(ctx, impl, priorErrors)
	// Emit reviewer-escalated event when MinPower is bumped above baseline.
	if priorErrors > 0 && r.BeadEvents != nil {
		_ = r.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
			Kind:      ReviewerEscalatedEventKind,
			Summary:   fmt.Sprintf("reviewer escalated to min_power=%d after %d prior error(s)", reviewProfile.MinPower, priorErrors),
			Body:      reviewerEscalatedEventBody(reviewProfile.MinPower, priorErrors, resultRev),
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})
	}

	out := &ReviewGroupResult{
		BeadID:    beadID,
		ResultRev: resultRev,
		Bundle: ReviewGroupBundle{
			GroupID:   groupID,
			DirAbs:    artifacts.DirAbs,
			DirRel:    artifacts.DirRel,
			PromptAbs: artifacts.PromptAbs,
			PromptRel: artifacts.PromptRel,
		},
		Slots: make([]ReviewGroupSlotResult, 0, 2),
	}

	var firstErr error
	for reviewerIndex := 0; reviewerIndex < 2; reviewerIndex++ {
		slotRuntime := BuildReviewGroupExecuteRequest(impl, reviewHarness, reviewProfile.Name, ReviewGroupDispatchMeta{
			GroupID:       groupID,
			ReviewerIndex: reviewerIndex,
		})
		// Apply escalated MinPower: use the higher of the base R4 floor and the
		// escalated profile floor so retries reach a stronger reviewer powerClass.
		if reviewProfile.MinPower > slotRuntime.MinPowerOverride {
			slotRuntime.MinPowerOverride = reviewProfile.MinPower
		}
		reviewRouteLabel := r.applyExplicitReviewerPins(&slotRuntime)
		slotRuntime.PromptFile = artifacts.PromptAbs
		slotRuntime.WorkDir = reviewWorkDir

		slotResult, slotErr := r.reviewGroupSlot(ctx, b, impl, resultRev, built, artifacts, reviewHarness, reviewRouteLabel, slotRuntime, caps.MaxPromptBytes)
		slot := ReviewGroupSlotResult{
			ReviewerIndex: reviewerIndex,
			Runtime:       slotRuntime,
		}
		if slotResult != nil {
			slotResult.ReviewerIndex = reviewerIndex
			slot.Result = slotResult
		}
		if slotErr != nil {
			slot.Error = slotErr.Error()
			if firstErr == nil {
				firstErr = slotErr
			}
		}
		out.Slots = append(out.Slots, slot)
	}

	// AC-check disagreement telemetry: when the reviewer's per-AC grade diverges
	// from the ac-check.json mechanical result, emit a review-ac-override event
	// for accuracy auditing. Best-effort — failures do not affect the outcome.
	if acCheckJSON != "" && r.BeadEvents != nil {
		for _, slot := range out.Slots {
			if slot.Result == nil {
				continue
			}
			count, reasons := countACGradeMismatches(acCheckJSON, slot.Result.PerAC)
			if count > 0 {
				_ = r.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
					Kind:      ReviewACOverrideEventKind,
					Summary:   fmt.Sprintf("%d AC grade(s) diverge from ac-check.json (reviewer_index=%d)", count, slot.ReviewerIndex),
					Body:      strings.Join(reasons, "\n"),
					Source:    "ddx work",
					CreatedAt: time.Now().UTC(),
				})
			}
		}
	}

	return out, firstErr
}

func (r *DefaultBeadReviewer) reviewGroupSlot(ctx context.Context, b *bead.Bead, impl ImplementerRouting, resultRev string, built BuildReviewPromptResult, artifacts *executeBeadArtifacts, reviewHarness, reviewModel string, runtime AgentRunRuntime, maxPromptBytes int) (*ReviewResult, error) {
	prompt := built.Prompt
	if built.Overflow {
		return &ReviewResult{
			Verdict:         VerdictBlock,
			Error:           evidence.OutcomeReviewContextOverflow,
			Rationale:       evidence.OutcomeReviewContextOverflow,
			ReviewerHarness: reviewHarness,
			ReviewerModel:   reviewModel,
			ResultRev:       resultRev,
			ExecutionDir:    artifacts.DirRel,
			CostUSD:         0,
			InputBytes:      len(prompt),
			OutputBytes:     0,
		}, fmt.Errorf("review-group: PROMPT_BUDGET_EXCEEDED/context_overflow (assembled prompt %d bytes exceeds cap %d; see %s)", len(prompt), maxPromptBytes, artifacts.DirRel)
	}

	start := time.Now()
	runRuntime := runtime
	runRuntime.Prompt = prompt
	result, runErr := r.dispatchReviewRun(ctx, runRuntime)
	durationMS := int(time.Since(start).Milliseconds())

	if runErr != nil {
		reviewRes := &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       runErr.Error(),
			Error:           evidence.OutcomeReviewTransport,
			ReviewerHarness: reviewHarness,
			ReviewerModel:   reviewModel,
			BaseRev:         resolveReviewBaseRev(r.ProjectRoot, resultRev),
			ResultRev:       resultRev,
			ExecutionDir:    artifacts.DirRel,
			DurationMS:      durationMS,
			CostUSD:         resultCost(result),
			InputBytes:      len(prompt),
			OutputBytes:     0,
		}
		return reviewRes, fmt.Errorf("review-group: %s: %w", evidence.OutcomeReviewTransport, runErr)
	}

	actualHarness := reviewHarness
	actualModel := reviewModel
	actualProvider := ""
	actualPower := 0
	if result != nil {
		if result.Harness != "" {
			actualHarness = result.Harness
		}
		if result.Model != "" {
			actualModel = result.Model
		}
		actualProvider = result.Provider
		actualPower = result.ActualPower
		durationMS = result.DurationMS
	}

	output := ""
	sessionID := ""
	if result != nil {
		output = result.Output
		sessionID = result.AgentSessionID
	}
	parsed, parseErr := ParseReviewVerdict([]byte(output))
	var strictVerdict Verdict
	var rationale string
	var findings []Finding
	if parseErr == nil {
		strictVerdict = parsed.Verdict
		rationale = strings.TrimSpace(parsed.Summary)
		findings = parsed.Findings
		if rationale == "" && len(findings) > 0 {
			parts := make([]string, 0, len(findings))
			for _, f := range findings {
				line := strings.TrimSpace(f.Summary)
				if line == "" {
					continue
				}
				if f.Location != "" {
					line = f.Location + ": " + line
				}
				parts = append(parts, line)
			}
			rationale = strings.Join(parts, "\n")
		}
	}
	baseRev := resolveReviewBaseRev(r.ProjectRoot, resultRev)
	reviewRes := &ReviewResult{
		Verdict:          strictVerdict,
		Rationale:        rationale,
		PerAC:            parsed.PerAC,
		Findings:         findings,
		ProseFindings:    parsed.ProseFindings,
		RawOutput:        output,
		ReviewerHarness:  actualHarness,
		ReviewerModel:    actualModel,
		ReviewerProvider: actualProvider,
		SessionID:        sessionID,
		BaseRev:          baseRev,
		ResultRev:        resultRev,
		ExecutionDir:     artifacts.DirRel,
		DurationMS:       durationMS,
		CostUSD:          resultCost(result),
		InputBytes:       len(prompt),
		OutputBytes:      len(output),
	}

	if parseErr != nil {
		class := evidence.OutcomeReviewUnparseable
		if strings.TrimSpace(output) == "" {
			class = evidence.OutcomeReviewProviderEmpty
		}
		reviewRes.Error = class
		return reviewRes, fmt.Errorf("review-group: %s: %w (raw output %d bytes; see %s)", class, parseErr, len(output), artifacts.DirRel)
	}
	if r.BeadEvents != nil && actualProvider != "" && impl.Provider != "" && actualProvider == impl.Provider {
		_ = r.BeadEvents.AppendEvent(b.ID, bead.BeadEvent{
			Kind:      ReviewPairingDegradedEventKind,
			Summary:   fmt.Sprintf("reviewer pinned to same provider as implementer (%s)", impl.Provider),
			Body:      reviewPairingDegradedBody(impl, actualHarness, actualProvider, actualModel, actualPower, resultRev),
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})
	}
	return reviewRes, nil
}
