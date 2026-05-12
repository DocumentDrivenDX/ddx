package agent

import (
	"context"
	"encoding/json"
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

type AutoRecoveryConfig struct {
	MaxRecoveryCostUSD float64
	MaxBeadCostUSD     float64
}

type autoRecoveryFailedEventBody struct {
	Reason       string  `json:"reason"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Detail       string  `json:"detail,omitempty"`
}

// PostLadderExhaustionHook is called when the consecutive_ladder_exhaustions
// counter for a bead reaches the auto-recovery threshold (>= 2). It should
// attempt automated recovery and return the outcome. A nil hook or a result
// with Attempted=false causes the caller to fall through to the existing loop
// path unchanged.
type PostLadderExhaustionHook func(ctx context.Context, beadID string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error)

// NewAutoRecoveryPostLadderExhaustionHook creates the production recovery hook.
// Persistent execution failures try reframe first and decompose second; too-large
// failures go straight to decompose; spec gaps use reframe only.
func NewAutoRecoveryPostLadderExhaustionHook(store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, cfg AutoRecoveryConfig) PostLadderExhaustionHook {
	return func(ctx context.Context, beadID string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		state := autoRecoveryState{store: store, beadID: beadID, cfg: cfg}
		switch failureClass {
		case SpecGap:
			return state.runReframe(ctx, store, runner, rcfg, projectRoot)
		case TooLarge:
			return state.runDecompose(ctx, store, runner, rcfg, projectRoot)
		default:
			reframeResult, err := state.runReframe(ctx, store, runner, rcfg, projectRoot)
			if err != nil || (reframeResult != nil && reframeResult.Succeeded) {
				return reframeResult, err
			}
			decomposeResult, err := state.runDecompose(ctx, store, runner, rcfg, projectRoot)
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
	total  float64
}

func (s *autoRecoveryState) runReframe(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string) (*PostLadderExhaustionResult, error) {
	result := runReframer(ctx, store, runner, rcfg, projectRoot, s.beadID)
	return s.recoveryResult(Reframe, result.Failed, result.CostUSD, result.Reason)
}

func (s *autoRecoveryState) runDecompose(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string) (*PostLadderExhaustionResult, error) {
	result := runDecomposer(ctx, store, runner, rcfg, projectRoot, s.beadID)
	return s.recoveryResult(Decompose, result.Failed, result.CostUSD, result.Reason)
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
		Reason:       reason,
		TotalCostUSD: s.total,
		Detail:       detail,
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
