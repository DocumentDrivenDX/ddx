package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	agentlib "github.com/easel/fizeau"
)

// drainWatchdog carries the three wedge-prevention mechanisms threaded into
// drainServiceEventsWithRenderer. All fields are optional: nil/zero disables
// the corresponding check.
type drainWatchdog struct {
	// cancel is called when a wedge condition is detected. Must not be nil
	// when idleTimeout or toolCallTimeout is non-zero.
	cancel func()
	// idleTimeout is the window of complete event silence before the execution
	// is considered wedged and cancelled. Any received event resets this timer;
	// only true silence (no events of any kind) triggers cancellation.
	idleTimeout time.Duration
	// toolCallTimeout is a per-tool-call sanity cap: if no tool_result arrives
	// within this window after a tool_call, the execution is cancelled. This
	// catches individually hung subprocesses, not loops (loopDetector handles
	// loops).
	toolCallTimeout time.Duration
}

// loopDetector maintains a window of the last 8 (tool_call, tool_result) pair
// keys and fires when the same key appears ≥4 times — indicating the model is
// repeating an unproductive command without progress.
type loopDetector struct {
	entries []string // capacity-capped at 8
}

// record adds key to the window and returns true if a loop is detected.
func (d *loopDetector) record(key string) bool {
	d.entries = append(d.entries, key)
	if len(d.entries) > 8 {
		d.entries = d.entries[len(d.entries)-8:]
	}
	counts := make(map[string]int, len(d.entries))
	for _, k := range d.entries {
		counts[k]++
		if counts[k] >= 4 {
			return true
		}
	}
	return false
}

// makeLoopKey builds a canonical key for a (tool_call, tool_result) pair.
// We use tool name + raw input JSON + first 256B of output + error prefix so
// that identical unproductive commands are detected regardless of surrounding
// whitespace or token formatting.
func makeLoopKey(call agentlib.ServiceToolCallData, result agentlib.ServiceToolResultData) string {
	input := strings.TrimSpace(string(call.Input))
	out := result.Output
	if len(out) > 256 {
		out = out[:256]
	}
	errStr := result.Error
	if len(errStr) > 64 {
		errStr = errStr[:64]
	}
	return call.Name + "\x00" + input + "\x00" + errStr + "\x00" + out
}

// runAgentViaService is the new RunAgent dispatch path that drives the
// agent through agentlib.FizeauService.Execute and drains the resulting event
// channel into a DDx Result. Old in-package code paths (RunAgent legacy
// loop, embeddedCompactionConfig, buildAgentProvider, findTool, and
// wrapProviderWithDeadlines) stay in place; this function does NOT call them.
//
// Stall detection: we delegate to the agent's StallPolicy. The agent
// emits a stall event then a final event with Status="stalled".
func runAgentViaService(r *Runner, opts RunArgs) (*Result, error) {
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

	providerTimeout := ResolveProviderRequestTimeout(wd, opts.Provider, model, 0)

	// Build the public ExecuteRequest per CONTRACT-003.
	req := agentlib.ServiceExecuteRequest{
		Prompt:          promptText,
		Model:           model,
		Provider:        opts.Provider,
		Harness:         "fiz",
		Reasoning:       agentlib.Reasoning(opts.Effort),
		Permissions:     opts.Permissions,
		WorkDir:         wd,
		Timeout:         wallClock,
		IdleTimeout:     timeout,
		ProviderTimeout: providerTimeout,
		SessionLogDir:   logDir,
		Metadata:        metadataWithEnv(opts.Correlation, opts.Env),
		Role:            opts.Role,
		CorrelationID:   opts.CorrelationID,
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

	watchdog := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     timeout,
		toolCallTimeout: time.Duration(ToolCallTimeout) * time.Millisecond,
	}
	onRouteResolved := func(harness, provider, model string) {
		harness = firstNonEmpty(harness, fizeauHarness(strings.TrimSpace(opts.Harness)))
		provider = firstNonEmpty(provider, strings.TrimSpace(opts.Provider))
		model = firstNonEmpty(model, strings.TrimSpace(opts.Model))
		route := providerRouteLabel(provider, model)
		_, _ = reapSupersededProviderChildren(context.Background(), os.Getpid(), route, harness, time.Now().UTC())
	}
	final, routing, _ := drainServiceEventsWithRenderer(events, nil, NewWorkLogRenderer(WorkLogRendererOptions{WorkPhase: "do"}), watchdog, onRouteResolved)
	elapsed := time.Since(start)

	result := &Result{
		Harness:    "fiz",
		Model:      model,
		DurationMS: int(elapsed.Milliseconds()),
	}
	if routing != nil {
		result.Provider = routing.Provider
		if routing.Model != "" {
			result.Model = routing.Model
		}
		if routing.Harness != "" {
			result.Harness = routing.Harness
		}
		if power, speed, cost, source := selectedRoutingCandidateMetrics(routing); power > 0 || speed > 0 || cost > 0 || source != "" {
			result.PredictedPower = power
			result.PredictedSpeedTPS = speed
			result.PredictedCostUSDPer1kTokens = cost
			result.PredictedCostSource = source
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
			if final.Usage.CacheReadTokens != nil {
				result.CachedTokens = *final.Usage.CacheReadTokens
			}
			if final.Usage.TotalTokens != nil {
				result.Tokens = *final.Usage.TotalTokens
			}
		}
		if final.CostUSD > 0 {
			result.CostUSD = final.CostUSD
		}
		result.ExitCode = final.ExitCode
		result.Error = final.Error
		if final.RoutingActual != nil {
			if result.Provider == "" {
				result.Provider = final.RoutingActual.Provider
			}
			if final.RoutingActual.Model != "" {
				result.Model = final.RoutingActual.Model
			}
			if final.RoutingActual.Harness != "" {
				result.Harness = final.RoutingActual.Harness
			}
			if final.RoutingActual.Power > 0 {
				result.ActualPower = final.RoutingActual.Power
			}
		}
		if final.SessionLogPath != "" {
			// surface as session ID for downstream cross-reference (mirrors
			// the legacy path's AgentSessionID population).
			result.AgentSessionID = final.SessionLogPath
		}
	}
	result.Error = appendProviderTimeoutHint(result.Error, providerTimeout)
	normalizeServiceFinalExitCode(result)

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

// drainServiceEventsWithRenderer drains the event stream and returns the
// aggregated final/routing/progress data. When wd is non-nil it activates
// three wedge-prevention mechanisms:
//
//  1. All-event idle reset: any received event resets the idle timer. The
//     timer fires only on complete silence (no events of any kind) within
//     the window.
//  2. Loop detection: identical (call, result) pairs that repeat ≥4 times in
//     an 8-entry window trigger wd.cancel.
//  3. Per-tool-call timeout: a tool_call without a matching tool_result within
//     wd.toolCallTimeout triggers wd.cancel.
func drainServiceEventsWithRenderer(events <-chan agentlib.ServiceEvent, w io.Writer, renderer WorkLogRenderer, wd *drainWatchdog, onRouteResolved func(harness, provider, model string)) (*agentlib.ServiceFinalData, *agentlib.ServiceRoutingDecisionData, []agentlib.ServiceProgressData) {
	var final *agentlib.ServiceFinalData
	var routing *agentlib.ServiceRoutingDecisionData
	var progress []agentlib.ServiceProgressData

	// Fast path: no watchdog, simple range loop with no timer overhead.
	if wd == nil || (wd.idleTimeout == 0 && wd.toolCallTimeout == 0) {
		for ev := range events {
			decoded, err := agentlib.DecodeServiceEvent(ev)
			if err != nil {
				continue
			}
			switch {
			case decoded.RoutingDecision != nil:
				routing = decoded.RoutingDecision
				if onRouteResolved != nil {
					onRouteResolved(routing.Harness, routing.Provider, routing.Model)
				}
				if w != nil {
					if line := renderer.at(ev.Time).FormatRoutingDecision(decoded.RoutingDecision); line != "" {
						_, _ = fmt.Fprint(w, line)
					}
				}
			case decoded.Progress != nil:
				progress = append(progress, *decoded.Progress)
				if w != nil {
					_, _ = fmt.Fprint(w, renderer.at(ev.Time).FormatServiceProgressEntries([]agentlib.ServiceProgressData{*decoded.Progress}))
				}
			case decoded.Final != nil:
				final = decoded.Final
			}
		}
		return final, routing, progress
	}

	// Watchdog path: select loop so timers can fire alongside event reads.
	ld := &loopDetector{}
	var pendingCall *agentlib.ServiceToolCallData

	// resetIdle stops and restarts the idle timer. Only called on meaningful events.
	var idleTimer *time.Timer
	var idleTimerC <-chan time.Time
	resetIdle := func() {
		if wd.idleTimeout <= 0 {
			return
		}
		if idleTimer == nil {
			idleTimer = time.NewTimer(wd.idleTimeout)
			idleTimerC = idleTimer.C
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(wd.idleTimeout)
	}

	var toolCallTimer *time.Timer
	var toolCallTimerC <-chan time.Time
	startToolCallTimer := func() {
		if wd.toolCallTimeout <= 0 {
			return
		}
		if toolCallTimer == nil {
			toolCallTimer = time.NewTimer(wd.toolCallTimeout)
			toolCallTimerC = toolCallTimer.C
			return
		}
		if !toolCallTimer.Stop() {
			select {
			case <-toolCallTimer.C:
			default:
			}
		}
		toolCallTimer.Reset(wd.toolCallTimeout)
	}
	stopToolCallTimer := func() {
		if toolCallTimer == nil {
			return
		}
		if !toolCallTimer.Stop() {
			select {
			case <-toolCallTimer.C:
			default:
			}
		}
		toolCallTimerC = nil
	}

	// Arm the idle timer immediately so a session that never starts a tool
	// call is still bounded.
	if wd.idleTimeout > 0 {
		idleTimer = time.NewTimer(wd.idleTimeout)
		idleTimerC = idleTimer.C
	}
	defer func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
		if toolCallTimer != nil {
			toolCallTimer.Stop()
		}
	}()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return final, routing, progress
			}
			decoded, err := agentlib.DecodeServiceEvent(ev)
			if err != nil {
				continue
			}
			// Any received event resets the idle timer; only true silence fires it.
			resetIdle()
			switch {
			case decoded.ToolCall != nil:
				startToolCallTimer()
				callCopy := *decoded.ToolCall
				pendingCall = &callCopy

			case decoded.ToolResult != nil:
				stopToolCallTimer()
				if pendingCall != nil {
					key := makeLoopKey(*pendingCall, *decoded.ToolResult)
					if ld.record(key) {
						_, _ = fmt.Fprintf(os.Stderr, "agent: loop detected (command=%q repeated ≥4 times): cancelling\n", pendingCall.Name)
						if wd.cancel != nil {
							wd.cancel()
						}
						return final, routing, progress
					}
					pendingCall = nil
				}

			case decoded.Final != nil:
				stopToolCallTimer()
				final = decoded.Final

			case decoded.RoutingDecision != nil:
				routing = decoded.RoutingDecision
				if onRouteResolved != nil {
					onRouteResolved(routing.Harness, routing.Provider, routing.Model)
				}
				if w != nil {
					if line := renderer.at(ev.Time).FormatRoutingDecision(decoded.RoutingDecision); line != "" {
						_, _ = fmt.Fprint(w, line)
					}
				}

			case decoded.Progress != nil:
				progress = append(progress, *decoded.Progress)
				if w != nil {
					_, _ = fmt.Fprint(w, renderer.at(ev.Time).FormatServiceProgressEntries([]agentlib.ServiceProgressData{*decoded.Progress}))
				}
			}

		case <-idleTimerC:
			_, _ = fmt.Fprintf(os.Stderr, "agent: idle timeout (%s without meaningful event): cancelling\n", wd.idleTimeout)
			if wd.cancel != nil {
				wd.cancel()
			}
			return final, routing, progress

		case <-toolCallTimerC:
			name := ""
			if pendingCall != nil {
				name = pendingCall.Name
			}
			_, _ = fmt.Fprintf(os.Stderr, "agent: tool call timeout (%s, tool=%q): cancelling\n", wd.toolCallTimeout, name)
			if wd.cancel != nil {
				wd.cancel()
			}
			return final, routing, progress
		}
	}
}

func selectedRoutingCandidateMetrics(routing *agentlib.ServiceRoutingDecisionData) (int, float64, float64, string) {
	if routing == nil {
		return 0, 0, 0, ""
	}
	for _, candidate := range routing.Candidates {
		if candidate.Eligible && candidate.Model == routing.Model {
			return candidate.Components.Power, candidate.Components.SpeedTPS, candidate.CostUSDPer1kTokens, candidate.CostSource
		}
	}
	return 0, 0, 0, ""
}
