package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// RecoveryFailureClass categorises the persistent failure mode that exhausted
// the escalation ladder, so the PostLadderExhaustionHook can choose the
// appropriate recovery action.
type RecoveryFailureClass string

const (
	// SpecGap is set when the last attempt was blocked by a spec-gap or
	// missing-acceptance review verdict.
	SpecGap RecoveryFailureClass = "spec_gap"
	// TooLarge is set when the last attempt was blocked by a too-large review
	// verdict.
	TooLarge RecoveryFailureClass = "too_large"
	// PersistentExecutionFailed is the default class for all other persistent
	// failures.
	PersistentExecutionFailed RecoveryFailureClass = "persistent_execution_failed"
)

// RecoveryPath indicates which automated recovery action a
// PostLadderExhaustionHook took.
type RecoveryPath string

const (
	Reframe   RecoveryPath = "reframe"
	Decompose RecoveryPath = "decompose"
)

// PostLadderExhaustionResult is the outcome returned by a
// PostLadderExhaustionHook invocation.
type PostLadderExhaustionResult struct {
	Attempted     bool
	Succeeded     bool
	Path          RecoveryPath
	CostUSD       float64
	OutcomeReason string
}

// PostLadderExhaustionContext carries the exhausted review evidence into TD-031
// recovery so the recovery action can stay linked to the review group and
// reviewer findings that triggered it.
type PostLadderExhaustionContext struct {
	ReviewGroupID        string
	ReviewVerdict        string
	ReviewRationale      string
	ReviewClassification string
	ReviewPerAC          []ReviewAC
	ReviewFindings       []Finding
	PreserveRef          string
}

func postLadderExhaustionContextFromReport(report ExecuteBeadReport) PostLadderExhaustionContext {
	return PostLadderExhaustionContext{
		ReviewGroupID:        strings.TrimSpace(report.ReviewGroupID),
		ReviewVerdict:        strings.TrimSpace(report.ReviewVerdict),
		ReviewRationale:      strings.TrimSpace(report.ReviewRationale),
		ReviewClassification: strings.TrimSpace(report.ReviewClassification),
		ReviewPerAC:          append([]ReviewAC(nil), report.ReviewPerAC...),
		ReviewFindings:       append([]Finding(nil), report.ReviewFindings...),
		PreserveRef:          strings.TrimSpace(report.PreserveRef),
	}
}

type AutoRecoveryConfig struct {
	MaxRecoveryCostUSD float64
	MaxBeadCostUSD     float64
}

type repairCycleExhaustedEventBody struct {
	ReviewGroupID        string                `json:"review_group_id,omitempty"`
	ReviewVerdict        string                `json:"review_verdict,omitempty"`
	ReviewRationale      string                `json:"review_rationale,omitempty"`
	ReviewClassification string                `json:"review_classification,omitempty"`
	ReviewPerAC          []ReviewAC            `json:"review_per_ac,omitempty"`
	ReviewFindings       []Finding             `json:"review_findings,omitempty"`
	BaseRev              string                `json:"base_rev,omitempty"`
	ResultRev            string                `json:"result_rev,omitempty"`
	CandidateRef         string                `json:"candidate_ref,omitempty"`
	PreserveRef          string                `json:"preserve_ref,omitempty"`
	RepairCycleCount     int                   `json:"repair_cycle_count,omitempty"`
	RecoveryAction       string                `json:"recovery_action,omitempty"`
	CycleTrace           []ExecutionCycleTrace `json:"cycle_trace,omitempty"`
	OutcomeReason        string                `json:"outcome_reason,omitempty"`
}

func appendRepairCycleExhaustedEvent(store ExecuteBeadLoopStore, beadID, actor string, at time.Time, report ExecuteBeadReport) {
	if store == nil || beadID == "" {
		return
	}
	body, err := json.Marshal(repairCycleExhaustedEventBody{
		ReviewGroupID:        strings.TrimSpace(report.ReviewGroupID),
		ReviewVerdict:        strings.TrimSpace(report.ReviewVerdict),
		ReviewRationale:      strings.TrimSpace(report.ReviewRationale),
		ReviewClassification: strings.TrimSpace(report.ReviewClassification),
		ReviewPerAC:          append([]ReviewAC(nil), report.ReviewPerAC...),
		ReviewFindings:       append([]Finding(nil), report.ReviewFindings...),
		BaseRev:              strings.TrimSpace(report.BaseRev),
		ResultRev:            strings.TrimSpace(report.ResultRev),
		CandidateRef:         strings.TrimSpace(report.CandidateRef),
		PreserveRef:          strings.TrimSpace(report.PreserveRef),
		RepairCycleCount:     report.RepairCycleCount,
		RecoveryAction:       strings.TrimSpace(report.RecoveryAction),
		CycleTrace:           append([]ExecutionCycleTrace(nil), report.CycleTrace...),
		OutcomeReason:        ExecuteBeadStatusRepairCycleExhausted,
	})
	if err != nil {
		body = []byte(fmt.Sprintf(
			"review_group_id=%s\nreview_verdict=%s\nreview_rationale=%s\nreview_classification=%s\noutcome_reason=%s",
			strings.TrimSpace(report.ReviewGroupID),
			strings.TrimSpace(report.ReviewVerdict),
			strings.TrimSpace(report.ReviewRationale),
			strings.TrimSpace(report.ReviewClassification),
			ExecuteBeadStatusRepairCycleExhausted,
		))
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      ExecuteBeadStatusRepairCycleExhausted,
		Summary:   ExecuteBeadStatusRepairCycleExhausted,
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

type autoRecoveryFailedEventBody struct {
	Reason               string     `json:"reason"`
	TotalCostUSD         float64    `json:"total_cost_usd"`
	Detail               string     `json:"detail,omitempty"`
	PreserveRef          string     `json:"preserve_ref,omitempty"`
	ReviewGroupID        string     `json:"review_group_id,omitempty"`
	ReviewVerdict        string     `json:"review_verdict,omitempty"`
	ReviewRationale      string     `json:"review_rationale,omitempty"`
	ReviewClassification string     `json:"review_classification,omitempty"`
	ReviewPerAC          []ReviewAC `json:"review_per_ac,omitempty"`
	ReviewFindings       []Finding  `json:"review_findings,omitempty"`
}

// PostLadderExhaustionHook is called when the consecutive_ladder_exhaustions
// counter for a bead reaches the auto-recovery threshold (>= 2). It should
// attempt automated recovery and return the outcome. A nil hook or a result
// with Attempted=false causes the caller to fall through to the existing loop
// path unchanged.
type PostLadderExhaustionHook func(ctx context.Context, beadID string, failureClass RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error)

// NewAutoRecoveryPostLadderExhaustionHook creates the production recovery hook.
// Persistent execution failures try reframe first and decompose second; too-large
// failures go straight to decompose; spec gaps use reframe only.
func NewAutoRecoveryPostLadderExhaustionHook(store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, cfg AutoRecoveryConfig) PostLadderExhaustionHook {
	return func(ctx context.Context, beadID string, failureClass RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
		state := autoRecoveryState{store: store, beadID: beadID, cfg: cfg, review: review}
		switch failureClass {
		case SpecGap:
			return state.runReframe(ctx, store, runner, rcfg, projectRoot, failureClass)
		case TooLarge:
			return state.runDecompose(ctx, store, runner, rcfg, projectRoot, failureClass)
		default:
			reframeResult, err := state.runReframe(ctx, store, runner, rcfg, projectRoot, failureClass)
			if err != nil || (reframeResult != nil && reframeResult.Succeeded) {
				return reframeResult, err
			}
			decomposeResult, err := state.runDecompose(ctx, store, runner, rcfg, projectRoot, failureClass)
			if err != nil || (decomposeResult != nil && decomposeResult.Succeeded) {
				return decomposeResult, err
			}
			if decomposeResult != nil && decomposeResult.OutcomeReason != "" {
				return decomposeResult, nil
			}
			return state.parkFailed("both_failed", "")
		}
	}
}

type autoRecoveryState struct {
	store  ExecuteBeadLoopStore
	beadID string
	cfg    AutoRecoveryConfig
	review PostLadderExhaustionContext
	total  float64
}

func (s *autoRecoveryState) runReframe(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
	hook := NewReframePostLadderExhaustionHook(store, runner, rcfg, projectRoot)
	result, err := hook(ctx, s.beadID, failureClass, s.review)
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Attempted {
		return result, nil
	}
	return s.recoveryResult(result.Path, !result.Succeeded, result.CostUSD, result.OutcomeReason)
}

func (s *autoRecoveryState) runDecompose(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
	hook := NewDecomposePostLadderExhaustionHook(store, runner, rcfg, projectRoot)
	// Force the TooLarge branch so the decomposer constructor stays on the live
	// recovery path even when we are falling back from a reframer failure.
	result, err := hook(ctx, s.beadID, TooLarge, s.review)
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Attempted {
		return result, nil
	}
	return s.recoveryResult(result.Path, !result.Succeeded, result.CostUSD, result.OutcomeReason)
}

func (s *autoRecoveryState) recoveryResult(path RecoveryPath, failed bool, costUSD float64, reason string) (*PostLadderExhaustionResult, error) {
	s.total += costUSD
	if detail, tripped := s.perBeadBudgetTripped(); tripped {
		_ = s.store.AppendEvent(s.beadID, bead.BeadEvent{
			Kind:      "per-bead-budget-exhausted",
			Summary:   "per-bead cost budget exhausted during automated recovery",
			Body:      detail,
			Actor:     "ddx work",
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})
		return &PostLadderExhaustionResult{
			Attempted:     true,
			Succeeded:     false,
			Path:          path,
			CostUSD:       s.total,
			OutcomeReason: escalation.PerBeadBudgetExhaustedReason,
		}, nil
	}
	if s.maxRecoveryCostTripped() {
		return s.parkFailed("circuit-breaker", reason)
	}
	return &PostLadderExhaustionResult{
		Attempted: true,
		Succeeded: !failed,
		Path:      path,
		CostUSD:   s.total,
	}, nil
}

func (s *autoRecoveryState) perBeadBudgetTripped() (string, bool) {
	if s.cfg.MaxBeadCostUSD <= 0 || s.total < s.cfg.MaxBeadCostUSD {
		return "", false
	}
	return escalation.PerBeadBudgetExhaustedReason + " during automated recovery", true
}

func (s *autoRecoveryState) maxRecoveryCostTripped() bool {
	return s.cfg.MaxRecoveryCostUSD > 0 && s.total > s.cfg.MaxRecoveryCostUSD
}

func (s *autoRecoveryState) parkFailed(reason, detail string) (*PostLadderExhaustionResult, error) {
	body, _ := json.Marshal(autoRecoveryFailedEventBody{
		Reason:               reason,
		TotalCostUSD:         s.total,
		Detail:               detail,
		PreserveRef:          strings.TrimSpace(s.review.PreserveRef),
		ReviewGroupID:        strings.TrimSpace(s.review.ReviewGroupID),
		ReviewVerdict:        strings.TrimSpace(s.review.ReviewVerdict),
		ReviewRationale:      strings.TrimSpace(s.review.ReviewRationale),
		ReviewClassification: strings.TrimSpace(s.review.ReviewClassification),
		ReviewPerAC:          append([]ReviewAC(nil), s.review.ReviewPerAC...),
		ReviewFindings:       append([]Finding(nil), s.review.ReviewFindings...),
	})
	_ = s.store.AppendEvent(s.beadID, bead.BeadEvent{
		Kind:      "auto-recovery-failed",
		Summary:   reason,
		Body:      string(body),
		Actor:     "ddx work",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})
	err := s.store.ParkToProposed(s.beadID, bead.ParkAutoRecoveryFailed, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason: reason,
			Since:  time.Now().UTC().Format(time.RFC3339),
			Source: "ddx work",
		})
	})
	return &PostLadderExhaustionResult{
		Attempted:     true,
		Succeeded:     false,
		CostUSD:       s.total,
		OutcomeReason: reason,
	}, err
}

// deriveRecoveryFailureClass maps the last-attempt report to a
// RecoveryFailureClass for use by the PostLadderExhaustionHook.
func deriveRecoveryFailureClass(report ExecuteBeadReport) RecoveryFailureClass {
	classification := strings.TrimSpace(report.ReviewClassification)
	switch classification {
	case ReviewTerminalClassSpecGap, ReviewTerminalClassMissingAcceptance:
		return SpecGap
	case ReviewTerminalClassTooLarge:
		return TooLarge
	}

	if classification == "" && (len(report.ReviewPerAC) > 0 || len(report.ReviewFindings) > 0 || report.ReviewVerdict != "") {
		derived := ClassifyReviewFindings(&ReviewResult{
			Verdict:   Verdict(strings.TrimSpace(report.ReviewVerdict)),
			Rationale: strings.TrimSpace(report.ReviewRationale),
			PerAC:     append([]ReviewAC(nil), report.ReviewPerAC...),
			Findings:  append([]Finding(nil), report.ReviewFindings...),
		})
		switch derived.Class {
		case ReviewTerminalClassSpecGap, ReviewTerminalClassMissingAcceptance:
			return SpecGap
		case ReviewTerminalClassTooLarge:
			return TooLarge
		}
	}

	switch {
	case strings.Contains(report.Status, ReviewTerminalClassSpecGap),
		strings.Contains(report.Status, ReviewTerminalClassMissingAcceptance):
		return SpecGap
	case strings.Contains(report.Status, ReviewTerminalClassTooLarge):
		return TooLarge
	default:
		return PersistentExecutionFailed
	}
}
