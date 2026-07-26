package agent

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
	agentlib "github.com/easel/fizeau"
)

// transitionRunRecordToRunning advances the pre-dispatch substrate to phase
// running when public Fizeau execution data first exists. No-op without
// attempt_id (plain ddx run). Fail-closed on write errors so readers never see
// a stuck dispatching record after a live public stream (ddx-a44bfc5b).
//
// public may be nil or empty: phase still becomes running because a typed
// public event existed; only non-empty public Fizeau contract fields are
// persisted under rec.Fizeau.
func transitionRunRecordToRunning(projectRoot string, runtime AgentRunRuntime, public *runrecord.FizeauPublicResult) error {
	corr := runtime.Correlation
	if corr == nil {
		return nil
	}
	attemptID := strings.TrimSpace(corr["attempt_id"])
	if attemptID == "" {
		return nil
	}
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("agent: transition run record to running: empty project root")
	}
	if err := runrecord.TransitionToRunning(projectRoot, attemptID, public); err != nil {
		return fmt.Errorf("agent: transition run record to running: %w", err)
	}
	return nil
}

// fizeauPublicFromRouting maps public routing-decision fields into the durable
// FizeauPublicResult shape. Concrete harness/provider/model pins are intentionally
// not persisted on the run substrate (schema forbids routing policy fields).
func fizeauPublicFromRouting(routing *agentlib.ServiceRoutingDecisionData) *runrecord.FizeauPublicResult {
	if routing == nil {
		return nil
	}
	out := &runrecord.FizeauPublicResult{}
	if sid := strings.TrimSpace(routing.SessionID); sid != "" {
		out.PublicSessionRef = sid
	}
	return out
}

// fizeauPublicFromFinal maps public final-event fields into FizeauPublicResult.
// Only typed public contract fields are copied — no provider process metadata.
func fizeauPublicFromFinal(final *agentlib.ServiceFinalData) *runrecord.FizeauPublicResult {
	if final == nil {
		return nil
	}
	out := &runrecord.FizeauPublicResult{}
	if p := strings.TrimSpace(final.SessionLogPath); p != "" {
		out.SessionLogPath = p
	}
	if s := strings.TrimSpace(final.Status); s != "" {
		out.FinalStatus = s
	}
	exit := final.ExitCode
	out.FinalExitCode = &exit
	if final.DurationMS != 0 {
		d := final.DurationMS
		out.DurationMS = &d
	}
	return out
}

// fizeauPublicFromDecoded extracts public route/result fields from a typed
// decoded Fizeau service event. Progress/tool events signal that a public
// stream exists but carry no FizeauPublicResult fields — callers still
// transition phase to running with a nil/empty public payload.
func fizeauPublicFromDecoded(decoded agentlib.ServiceDecodedEvent) (public *runrecord.FizeauPublicResult, isPublicExecution bool) {
	switch {
	case decoded.RoutingDecision != nil:
		return fizeauPublicFromRouting(decoded.RoutingDecision), true
	case decoded.Final != nil:
		return fizeauPublicFromFinal(decoded.Final), true
	case decoded.Progress != nil,
		decoded.TextDelta != nil,
		decoded.ToolCall != nil,
		decoded.ToolResult != nil,
		decoded.Compaction != nil,
		decoded.Stall != nil,
		decoded.ContextCapacity != nil:
		return nil, true
	default:
		return nil, false
	}
}
