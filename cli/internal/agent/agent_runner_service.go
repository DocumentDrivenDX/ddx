package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// useNewAgentPath reports whether RunAgent should dispatch to the new
// agentlib.FizeauService.Execute path. Default is on. Set the env var
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
		Harness:         "agent",
		ModelRef:        opts.ModelRef,
		Reasoning:       agentlib.Reasoning(opts.Effort),
		Permissions:     opts.Permissions,
		WorkDir:         wd,
		Timeout:         wallClock,
		IdleTimeout:     timeout,
		ProviderTimeout: providerTimeout,
		SessionLogDir:   logDir,
		Metadata:        opts.Correlation,
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

	final, routing, _ := drainServiceEvents(events)
	elapsed := time.Since(start)

	result := &Result{
		Harness:    "agent",
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
// the routing decision (when present in the routing_decision start event), and
// any canonical progress payloads emitted by the service.
func drainServiceEvents(events <-chan agentlib.ServiceEvent) (*agentlib.ServiceFinalData, *agentlib.ServiceRoutingDecisionData, []agentlib.ServiceProgressData) {
	var final *agentlib.ServiceFinalData
	var routing *agentlib.ServiceRoutingDecisionData
	var progress []agentlib.ServiceProgressData

	for ev := range events {
		decoded, err := agentlib.DecodeServiceEvent(ev)
		if err != nil {
			continue
		}
		switch {
		case decoded.RoutingDecision != nil:
			routing = decoded.RoutingDecision
		case decoded.Progress != nil:
			progress = append(progress, *decoded.Progress)
		case decoded.Final != nil:
			final = decoded.Final
		}
	}
	return final, routing, progress
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
