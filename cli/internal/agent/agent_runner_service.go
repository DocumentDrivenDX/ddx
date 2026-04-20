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

// serviceFinalData mirrors the JSON shape of a CONTRACT-003 type=final
// event payload (harnesses.FinalData). Defined locally because
// internal/harnesses is module-private.
type serviceFinalData struct {
	Status         string                `json:"status"`
	ExitCode       int                   `json:"exit_code"`
	Error          string                `json:"error,omitempty"`
	DurationMS     int64                 `json:"duration_ms"`
	Usage          *serviceFinalUsage    `json:"usage,omitempty"`
	CostUSD        float64               `json:"cost_usd,omitempty"`
	SessionLogPath string                `json:"session_log_path,omitempty"`
	RoutingActual  *serviceRoutingActual `json:"routing_actual,omitempty"`
}

type serviceFinalUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type serviceRoutingActual struct {
	Harness            string   `json:"harness"`
	Provider           string   `json:"provider,omitempty"`
	Model              string   `json:"model"`
	FallbackChainFired []string `json:"fallback_chain_fired,omitempty"`
}

// serviceToolCallData mirrors harnesses.ToolCallData.
type serviceToolCallData struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// serviceToolResultData mirrors harnesses.ToolResultData.
type serviceToolResultData struct {
	ID         string `json:"id"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

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
		if final.Usage != nil {
			result.InputTokens = final.Usage.InputTokens
			result.OutputTokens = final.Usage.OutputTokens
			result.Tokens = final.Usage.TotalTokens
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
