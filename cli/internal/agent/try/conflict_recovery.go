package try

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const (
	StatusLandConflictUnresolvable = "land_conflict_unresolvable"
	StatusLandConflictNeedsHuman   = "land_conflict_needs_human"
)

type ConflictRecoveryDisposition int

const (
	ConflictRecoveryMerged ConflictRecoveryDisposition = iota
	ConflictRecoveryPark
	ConflictRecoveryNeedsHuman
)

type ConflictRecoveryInput struct {
	Bead             bead.Bead
	Report           Report
	ProjectRoot      string
	AutoRecover      ConflictAutoRecoverFn
	ConflictResolver ConflictResolverFn
	Store            Store
	Assignee         string
	Now              func() time.Time
	Cooldown         time.Duration
}

type ConflictRecoveryOutput struct {
	Report      Report
	Disposition ConflictRecoveryDisposition
	StoreErrOp  string
	StoreErr    error
}

func ConflictRecoveryOutcome(ctx context.Context, in ConflictRecoveryInput) Outcome {
	out := RunConflictRecovery(ctx, in)
	disposition := OutcomePark
	if out.Disposition == ConflictRecoveryMerged {
		disposition = OutcomeSuccess
	}
	return Outcome{
		Report:      out.Report,
		Disposition: disposition,
		StoreErrOp:  out.StoreErrOp,
		StoreErr:    out.StoreErr,
	}
}

func RunConflictRecovery(ctx context.Context, in ConflictRecoveryInput) ConflictRecoveryOutput {
	report := in.Report
	out := ConflictRecoveryOutput{Report: report}

	now := in.Now
	if now == nil {
		now = time.Now
	}

	autoFn := in.AutoRecover
	if autoFn == nil {
		autoFn = func(string, string) (string, error) {
			return "", fmt.Errorf("conflict auto-recover function is required")
		}
	}

	newTip, autoErr := autoFn(in.ProjectRoot, report.PreserveRef)
	if autoErr == nil && newTip != "" {
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "land-conflict-auto-recovered",
			Summary:   "preserved iteration auto-recovered onto current tip via ort",
			Body:      fmt.Sprintf("preserve_ref=%s\nnew_tip=%s", report.PreserveRef, newTip),
			Actor:     in.Assignee,
			Source:    "ddx agent try",
			CreatedAt: now().UTC(),
		})
		report.Status = StatusSuccess
		report.ResultRev = newTip
		report.Detail = "auto-recovered: merged preserved iteration onto current tip via ort"
		if err := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); err != nil {
			out.StoreErrOp = "CloseWithEvidence"
			out.StoreErr = err
			out.Report = report
			return out
		}
		out.Report = report
		out.Disposition = ConflictRecoveryMerged
		return out
	}

	if in.ConflictResolver != nil {
		resolvedTip, isBlocking, resolveErr := in.ConflictResolver(ctx, in.Bead.ID, report.PreserveRef, in.ProjectRoot)
		if resolveErr == nil && resolvedTip != "" {
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "land-conflict-auto-recovered",
				Summary:   "preserved iteration resolved by focused conflict-resolve agent",
				Body:      fmt.Sprintf("preserve_ref=%s\nnew_tip=%s", report.PreserveRef, resolvedTip),
				Actor:     in.Assignee,
				Source:    "ddx agent try",
				CreatedAt: now().UTC(),
			})
			report.Status = StatusSuccess
			report.ResultRev = resolvedTip
			report.Detail = "auto-recovered: focused conflict-resolve agent landed preserved iteration"
			if err := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); err != nil {
				out.StoreErrOp = "CloseWithEvidence"
				out.StoreErr = err
				out.Report = report
				return out
			}
			out.Report = report
			out.Disposition = ConflictRecoveryMerged
			return out
		}
		if isBlocking {
			report.Status = StatusLandConflictNeedsHuman
		} else {
			report.Status = StatusLandConflictUnresolvable
		}
	} else {
		report.Status = StatusLandConflictUnresolvable
	}

	autoErrMsg := ""
	if autoErr != nil {
		autoErrMsg = autoErr.Error()
	}
	body, mErr := json.Marshal(map[string]any{
		"preserve_ref":     report.PreserveRef,
		"base_rev":         report.BaseRev,
		"result_rev":       report.ResultRev,
		"session_id":       report.SessionID,
		"auto_merge_error": autoErrMsg,
	})
	bodyStr := report.PreserveRef
	if mErr == nil {
		bodyStr = string(body)
	}
	eventKind := "land-conflict-unresolvable"
	if report.Status == StatusLandConflictNeedsHuman {
		eventKind = "land-conflict-needs-human"
	}
	_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
		Kind:      eventKind,
		Summary:   "preserved iteration could not be auto-recovered; parked for operator",
		Body:      bodyStr,
		Actor:     in.Assignee,
		Source:    "ddx agent try",
		CreatedAt: now().UTC(),
	})
	report.Detail = report.Status + ": preserve_ref=" + report.PreserveRef

	if err := in.Store.Unclaim(in.Bead.ID); err != nil {
		out.StoreErrOp = "Unclaim"
		out.StoreErr = err
		out.Report = report
		return out
	}
	parkUntil := now().UTC().Add(in.Cooldown)
	if err := in.Store.SetExecutionCooldown(in.Bead.ID, parkUntil, report.Status, report.Detail); err != nil {
		out.StoreErrOp = "SetExecutionCooldown"
		out.StoreErr = err
		out.Report = report
		return out
	}
	report.RetryAfter = parkUntil.Format(time.RFC3339)

	out.Report = report
	if report.Status == StatusLandConflictNeedsHuman {
		out.Disposition = ConflictRecoveryNeedsHuman
	} else {
		out.Disposition = ConflictRecoveryPark
	}
	return out
}
