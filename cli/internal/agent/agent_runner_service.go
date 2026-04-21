package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
)

// Public event-type strings emitted by agentlib.DdxAgent.Execute, mirrored
// from CONTRACT-003 §"Event JSON shapes". Kept as constants here so the
// drain loop does not have to import the agent's internal/harnesses
// package (which is module-private).
const (
	serviceEventRoutingDecision = "routing_decision"
	serviceEventTextDelta       = "text_delta"
	serviceEventToolCall        = "tool_call"
	serviceEventToolResult      = "tool_result"
	serviceEventStall           = "stall"
	serviceEventFinal           = "final"
	serviceEventCompaction      = "compaction"
	serviceEventCompactionEnd   = "compaction.end"
)

const (
	serviceNoopCompactionWallClockLimit  = 15 * time.Minute
	serviceNoopCompactionWallClockReason = "compaction_stuck_wall_clock_timeout"
)

// ServiceEvent payload types are now aliases to the upstream v0.8.0 public
// types (agent-f0bc5467). DDx previously maintained byte-for-byte shadow
// copies of these structs because CONTRACT-003 hadn't published them; that
// left the consumer carrying stream-adjacent knowledge and drifted whenever
// upstream shapes changed. With v0.8.0 the types are public and stable.
//
// Aliases rather than an outright delete so existing call sites keep
// working without a sweep. New code can (and should) use agentlib types
// directly.
type (
	serviceFinalData      = agentlib.ServiceFinalData
	serviceFinalUsage     = agentlib.ServiceFinalUsage
	serviceRoutingActual  = agentlib.ServiceRoutingActual
	serviceToolCallData   = agentlib.ServiceToolCallData
	serviceToolResultData = agentlib.ServiceToolResultData
)

// useNewAgentPath reports whether RunAgent should dispatch to the new
// agentlib.DdxAgent.Execute path. Default is on. Set the env var
// DDX_USE_NEW_AGENT_PATH=0 (or "false") to disable as an emergency escape
// hatch.
func useNewAgentPath() bool {
	switch os.Getenv("DDX_USE_NEW_AGENT_PATH") {
	case "0", "false", "FALSE", "False":
		return false
	default:
		return true
	}
}

// runAgentViaService is the new RunAgent dispatch path that drives the
// agent through agentlib.DdxAgent.Execute and drains the resulting event
// channel into a DDx Result. Old in-package code paths (RunAgent legacy
// loop, embeddedCompactionConfig, buildAgentProvider, findTool,
// wrapProviderWithDeadlines, stall + compaction-stuck circuit breakers)
// stay in place; this function does NOT call them.
//
// Stall detection: we delegate to the agent's StallPolicy. The agent
// emits a stall event then a final event with Status="stalled".
func runAgentViaService(r *Runner, opts RunOptions) (*Result, error) {
	promptText, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}

	model := r.resolveModel(opts, "agent")
	timeout := r.resolveTimeout(opts)
	wallClock := r.resolveWallClock(opts)

	wd := opts.WorkDir
	if wd == "" {
		wd, _ = os.Getwd()
	}

	// Construct the service. Reuses NewServiceFromWorkDir so provider/model
	// routing data lands on the agent the same way every other DDx command
	// constructs it (see serviceconfig.go).
	svc, err := NewServiceFromWorkDir(wd)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}

	// Resolve where to write the per-request session log.
	logDir := opts.SessionLogDir
	if logDir == "" {
		logDir = r.Config.SessionLogDir
	}
	if logDir == "" {
		logDir = DefaultLogDir
	}

	// Build the public ExecuteRequest per CONTRACT-003.
	req := agentlib.ServiceExecuteRequest{
		Prompt:          promptText,
		Model:           model,
		Provider:        opts.Provider,
		Harness:         "agent",
		ModelRef:        opts.ModelRef,
		Reasoning:       agentlib.Reasoning(opts.Effort),
		Permissions:     opts.Permissions,
		WorkDir:         wd,
		Timeout:         wallClock,
		IdleTimeout:     timeout,
		ProviderTimeout: DefaultProviderRequestTimeout,
		SessionLogDir:   logDir,
		Metadata:        opts.Correlation,
	}

	parentCtx := opts.Context
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	start := time.Now()
	events, err := svc.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("agent: execute: %w", err)
	}

	final, toolCalls, routing := drainServiceEvents(events)
	elapsed := time.Since(start)

	result := &Result{
		Harness:    "agent",
		Model:      model,
		DurationMS: int(elapsed.Milliseconds()),
		ToolCalls:  toolCalls,
	}
	if routing != nil {
		result.Provider = routing.Provider
		if routing.Model != "" {
			result.Model = routing.Model
		}
	}
	if final != nil {
		// Normalized final text from the upstream harness (agent-32e8ff5e).
		result.Output = final.FinalText
		if final.Usage != nil {
			// v0.9.1: Usage fields became *int so the API can distinguish
			// "harness reported zero" from "harness didn't report". Treat
			// nil as zero for DDx's int-valued result struct.
			if final.Usage.InputTokens != nil {
				result.InputTokens = *final.Usage.InputTokens
			}
			if final.Usage.OutputTokens != nil {
				result.OutputTokens = *final.Usage.OutputTokens
			}
			if final.Usage.TotalTokens != nil {
				result.Tokens = *final.Usage.TotalTokens
			}
		}
		if final.CostUSD > 0 {
			result.CostUSD = final.CostUSD
		}
		if final.RoutingActual != nil {
			if result.Provider == "" {
				result.Provider = final.RoutingActual.Provider
			}
			if final.RoutingActual.Model != "" {
				result.Model = final.RoutingActual.Model
			}
		}
		switch final.Status {
		case "success", "":
			// happy path; no-op
		case "stalled":
			result.ExitCode = 1
			if final.Error != "" {
				result.Error = "stalled: " + final.Error
			} else {
				result.Error = "stalled"
			}
		case "timed_out":
			result.ExitCode = 1
			result.Error = fmt.Sprintf("timeout after %v", wallClock.Round(time.Second))
		case "cancelled":
			result.ExitCode = 1
			result.Error = "cancelled"
		default:
			result.ExitCode = 1
			if final.Error != "" {
				result.Error = final.Error
			} else {
				result.Error = final.Status
			}
		}
		if final.SessionLogPath != "" {
			// surface as session ID for downstream cross-reference (mirrors
			// the legacy path's AgentSessionID population).
			result.AgentSessionID = final.SessionLogPath
		}
	}

	promptSource := opts.PromptSource
	if promptSource == "" {
		if opts.PromptFile != "" {
			promptSource = opts.PromptFile
		} else {
			promptSource = "inline"
		}
	}
	r.logSession(result, len(promptText), promptText, promptSource, opts.Correlation)
	r.recordRoutingOutcome(result, elapsed, opts)
	return result, nil
}

// drainServiceEvents reads service events and returns the final-event payload,
// the accumulated tool-call log, and the routing decision (when present in the
// routing_decision start event). A sustained run of no-op compaction telemetry
// is converted into a synthetic stalled final so execute-bead result details
// identify the time-based breaker instead of waiting for the outer wall clock.
func drainServiceEvents(events <-chan agentlib.ServiceEvent) (*serviceFinalData, []ToolCallEntry, *serviceRoutingActual) {
	var final *serviceFinalData
	var routing *serviceRoutingActual
	var toolCalls []ToolCallEntry
	pending := make(map[string]*ToolCallEntry) // call_id -> entry awaiting result
	var noopCompactions serviceNoopCompactionStreak
	var noopTimer *time.Timer
	var noopTimerC <-chan time.Time
	defer func() { stopNoopCompactionTimer(noopTimer) }()

	for {
		select {
		case <-noopTimerC:
			detail := noopCompactions.detail(serviceNoopCompactionWallClockLimit, serviceNoopCompactionWallClockLimit)
			return &serviceFinalData{
				Status: "stalled",
				Error:  detail,
			}, toolCallsWithPending(toolCalls, pending), routing
		case ev, ok := <-events:
			if !ok {
				// Any tool_call without a matching tool_result still gets recorded.
				return final, toolCallsWithPending(toolCalls, pending), routing
			}
			if isNoopCompactionEvent(ev) {
				detail, started := noopCompactions.record(eventTimestamp(ev), serviceNoopCompactionWallClockLimit)
				if started {
					noopTimer, noopTimerC = resetNoopCompactionTimer(noopTimer, serviceNoopCompactionWallClockLimit)
				}
				if detail != "" {
					return &serviceFinalData{
						Status: "stalled",
						Error:  detail,
					}, toolCallsWithPending(toolCalls, pending), routing
				}
				continue
			}
			if isServiceProgressEvent(ev) {
				noopCompactions.reset()
				stopNoopCompactionTimer(noopTimer)
				noopTimerC = nil
			}

			switch string(ev.Type) {
			case serviceEventRoutingDecision:
				var payload struct {
					Harness  string `json:"harness"`
					Provider string `json:"provider"`
					Model    string `json:"model"`
				}
				if err := json.Unmarshal(ev.Data, &payload); err == nil {
					routing = &serviceRoutingActual{
						Harness:  payload.Harness,
						Provider: payload.Provider,
						Model:    payload.Model,
					}
				}
			case serviceEventToolCall:
				var data serviceToolCallData
				if err := json.Unmarshal(ev.Data, &data); err == nil {
					entry := &ToolCallEntry{
						Tool:  data.Name,
						Input: string(data.Input),
					}
					pending[data.ID] = entry
				}
			case serviceEventToolResult:
				var data serviceToolResultData
				if err := json.Unmarshal(ev.Data, &data); err == nil {
					if entry, ok := pending[data.ID]; ok {
						entry.Output = data.Output
						entry.Error = data.Error
						entry.Duration = int(data.DurationMS)
						toolCalls = append(toolCalls, *entry)
						delete(pending, data.ID)
					}
				}
			case serviceEventFinal:
				var data serviceFinalData
				if err := json.Unmarshal(ev.Data, &data); err == nil {
					final = &data
				}
			}
		}
	}
}

func resetNoopCompactionTimer(timer *time.Timer, limit time.Duration) (*time.Timer, <-chan time.Time) {
	if limit <= 0 {
		return timer, nil
	}
	if timer == nil {
		timer = time.NewTimer(limit)
		return timer, timer.C
	}
	stopNoopCompactionTimer(timer)
	timer.Reset(limit)
	return timer, timer.C
}

func stopNoopCompactionTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

type serviceNoopCompactionStreak struct {
	start time.Time
	count int
}

func (s *serviceNoopCompactionStreak) record(ts time.Time, limit time.Duration) (string, bool) {
	if limit <= 0 {
		return "", false
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	started := false
	if s.count == 0 {
		s.start = ts
		started = true
	}
	s.count++
	elapsed := ts.Sub(s.start)
	if elapsed < limit {
		return "", started
	}
	return s.detail(elapsed, limit), started
}

func (s *serviceNoopCompactionStreak) detail(elapsed, limit time.Duration) string {
	if elapsed < limit {
		elapsed = limit
	}
	return fmt.Sprintf("%s: time-based breaker fired after %s of consecutive no-op compaction events (limit %s, count %d)",
		serviceNoopCompactionWallClockReason,
		elapsed.Round(time.Second),
		limit.Round(time.Second),
		s.count)
}

func (s *serviceNoopCompactionStreak) reset() {
	s.start = time.Time{}
	s.count = 0
}

func eventTimestamp(ev agentlib.ServiceEvent) time.Time {
	if !ev.Time.IsZero() {
		return ev.Time
	}
	return time.Now().UTC()
}

func isNoopCompactionEvent(ev agentlib.ServiceEvent) bool {
	switch string(ev.Type) {
	case serviceEventCompaction, serviceEventCompactionEnd:
	default:
		return false
	}
	var payload struct {
		NoCompaction bool `json:"no_compaction"`
	}
	if err := json.Unmarshal(ev.Data, &payload); err != nil {
		return false
	}
	return payload.NoCompaction
}

func isServiceProgressEvent(ev agentlib.ServiceEvent) bool {
	switch string(ev.Type) {
	case serviceEventTextDelta, serviceEventToolCall, serviceEventToolResult, serviceEventCompaction, serviceEventCompactionEnd:
		return true
	default:
		return false
	}
}

func toolCallsWithPending(toolCalls []ToolCallEntry, pending map[string]*ToolCallEntry) []ToolCallEntry {
	for _, entry := range pending {
		toolCalls = append(toolCalls, *entry)
	}
	return toolCalls
}
