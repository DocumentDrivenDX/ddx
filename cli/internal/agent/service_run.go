package agent

// service_run.go provides Runner-free helpers that drive every operation via
// the agentlib.DdxAgent service surface (CONTRACT-003). RunWithConfigViaService
// is the only run entry point; legacy RunViaService/RunViaServiceWith and the
// runFixtureHarnessViaRunner shim were retired in B22d-h.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// appendProviderTimeoutHint appends a configuration override hint to errMsg
// when it indicates a provider request timeout fired. Helps operators find
// the knob to adjust (AC4 of ddx-2c63bb95).
func appendProviderTimeoutHint(errMsg string, providerTimeout time.Duration) string {
	if errMsg == "" || !strings.Contains(errMsg, "provider request timeout") {
		return errMsg
	}
	return errMsg + fmt.Sprintf(
		"; request_timeout=%s exceeded — override via"+
			" agent.endpoints.<name>.request_timeout_seconds in .ddx/config.yaml"+
			" or pass --request-timeout DURATION to execute-bead/execute-loop",
		providerTimeout.Round(time.Second),
	)
}

// serviceRunFactory, when non-nil, overrides NewServiceFromWorkDir inside
// RunWithConfigViaService. CLI-level integration tests inject a stub to
// observe ServiceExecuteRequest fields without a real agent server.
var serviceRunFactory func(workDir string) (agentlib.DdxAgent, error)

// SetServiceRunFactory installs a service factory for RunWithConfigViaService.
// Pass nil to restore production behavior. Exported for cmd/ integration tests.
func SetServiceRunFactory(f func(workDir string) (agentlib.DdxAgent, error)) {
	serviceRunFactory = f
}

// RunWithConfigViaService dispatches a single agent invocation through the
// agent service. It takes a sealed ResolvedConfig (durable knobs) and an
// AgentRunRuntime (per-invocation plumbing/intent), and is the SD-024
// successor to the retired RunViaService/RunViaServiceWith entry points.
//
// The virtual and script harnesses route through a DDx-owned Runner path,
// not the upstream service. These are not "carve-outs pending migration" —
// they are different products from upstream's same-named stubs. DDx's
// virtual is a content-addressed record/replay dictionary keyed by
// PromptHash; upstream's is a unit-test stub where callers stuff
// virtual.response into Metadata. DDx's script reads a filesystem directive
// file; upstream does not model this at all.
func RunWithConfigViaService(ctx context.Context, workDir string, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	harness := rcfg.Passthrough().Harness

	if harness == "virtual" || harness == "script" {
		sessionLogDir := runtime.SessionLogDirOverride
		if sessionLogDir == "" {
			sessionLogDir = rcfg.SessionLogDir()
		}
		cfg := Config{SessionLogDir: ResolveLogDir(workDir, "")}
		r := NewRunner(cfg)
		r.WorkDir = workDir
		return r.runInternal(RunArgs{
			Context:       ctx,
			Harness:       rcfg.Harness(),
			Prompt:        runtime.Prompt,
			PromptFile:    runtime.PromptFile,
			PromptSource:  runtime.PromptSource,
			Correlation:   runtime.Correlation,
			Model:         rcfg.Model(),
			Provider:      rcfg.Provider(),
			ModelRef:      rcfg.ModelRef(),
			Effort:        rcfg.Effort(),
			Timeout:       rcfg.Timeout(),
			WallClock:     rcfg.WallClock(),
			WorkDir:       runtime.WorkDir,
			Permissions:   rcfg.Permissions(),
			SessionLogDir: sessionLogDir,
		})
	}

	factory := serviceRunFactory
	if factory == nil {
		factory = NewServiceFromWorkDir
	}
	svc, err := factory(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}
	return executeOnService(ctx, svc, workDir, rcfg, runtime)
}

// executeOnService dispatches against a pre-built svc using the durable
// knobs from rcfg and per-invocation plumbing from runtime, then records
// one session-index row. It is the inlined replacement for the retired
// RunViaServiceWith helper and is shared with dispatchViaResolvedConfig.
func executeOnService(ctx context.Context, svc agentlib.DdxAgent, workDir string, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	if svc == nil {
		return nil, fmt.Errorf("agent: nil service")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	promptText := runtime.Prompt
	if runtime.PromptFile != "" {
		data, err := readPromptFileBounded(runtime.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("agent: read prompt file: %w", err)
		}
		promptText = string(data)
	}
	if promptText == "" {
		return nil, fmt.Errorf("agent: prompt is required")
	}

	wd := runtime.WorkDir
	if wd == "" {
		wd = workDir
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}

	idle := rcfg.Timeout()
	if idle <= 0 {
		idle = time.Duration(DefaultTimeoutMS) * time.Millisecond
	}
	wall := rcfg.WallClock()
	if wall <= 0 {
		wall = time.Duration(DefaultWallClockMS) * time.Millisecond
	}

	sessionLogDir := runtime.SessionLogDirOverride
	if sessionLogDir == "" {
		sessionLogDir = rcfg.SessionLogDir()
	}
	if sessionLogDir == "" {
		sessionLogDir = ResolveLogDir(workDir, "")
	}

	pt := rcfg.Passthrough()

	harness := runtime.HarnessOverride
	if harness == "" {
		harness = pt.Harness
	}

	model := runtime.ModelOverride
	if model == "" {
		model = pt.Model
	}

	permissions := runtime.PermissionsOverride
	if permissions == "" {
		permissions = rcfg.Permissions()
	}

	providerTimeout := ResolveProviderRequestTimeout(workDir, pt.Provider, model, rcfg.ProviderRequestTimeout())

	req := agentlib.ServiceExecuteRequest{
		Prompt:          promptText,
		Model:           model,
		Profile:         rcfg.Profile(),
		Provider:        pt.Provider,
		Harness:         harness,
		ModelRef:        rcfg.ModelRef(),
		Reasoning:       agentlib.Reasoning(rcfg.Effort()),
		Permissions:     permissions,
		WorkDir:         wd,
		Timeout:         wall,
		IdleTimeout:     idle,
		ProviderTimeout: providerTimeout,
		SessionLogDir:   sessionLogDir,
		Metadata:        runtime.Correlation,
		MinPower:        rcfg.MinPower(),
		MaxPower:        rcfg.MaxPower(),
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	start := time.Now().UTC()
	events, err := svc.Execute(cancelCtx, req)
	if err != nil {
		return nil, fmt.Errorf("agent: execute: %w", err)
	}

	final, toolCalls, routing, actualPower := drainServiceEvents(events)
	finishedAt := time.Now().UTC()
	elapsed := finishedAt.Sub(start)

	result := &Result{
		Harness:    harness,
		Model:      model,
		DurationMS: int(elapsed.Milliseconds()),
		ToolCalls:  toolCalls,
	}
	if routing != nil {
		result.Provider = routing.Provider
		if routing.Model != "" {
			result.Model = routing.Model
		}
		if routing.Harness != "" {
			result.Harness = routing.Harness
		}
	}
	if actualPower > 0 {
		result.ActualPower = actualPower
	}
	if final != nil {
		// Normalized final text from the upstream harness (agent-32e8ff5e);
		// reviewer verdict extraction now parses this instead of raw stream
		// frames (ddx-7bc0c8d5).
		result.Output = final.FinalText
		if final.Usage != nil {
			// v0.9.1: Usage fields became *int (nullable).
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
			// happy path
		case "stalled":
			result.ExitCode = 1
			if final.Error != "" {
				result.Error = "stalled: " + final.Error
			} else {
				result.Error = "stalled"
			}
		case "timed_out":
			result.ExitCode = 1
			result.Error = fmt.Sprintf("timeout after %v", wall.Round(time.Second))
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
		result.Error = appendProviderTimeoutHint(result.Error, providerTimeout)
		if final.SessionLogPath != "" {
			result.AgentSessionID = final.SessionLogPath
		}
	}
	entry := SessionIndexEntryFromResult(workDir, SessionIndexInputs{
		Harness:     harness,
		Model:       model,
		Provider:    pt.Provider,
		Effort:      rcfg.Effort(),
		Correlation: runtime.Correlation,
	}, result, start, finishedAt)
	_ = AppendSessionIndex(ResolveLogDir(workDir, ""), entry, finishedAt)
	return result, nil
}

// CapabilitiesViaService returns HarnessCapabilities for the named harness by
// querying the service's ListHarnesses and (best-effort) the harness registry.
// It is the production replacement for Runner.Capabilities.
func CapabilitiesViaService(ctx context.Context, workDir, harnessName string) (*HarnessCapabilities, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := NewServiceFromWorkDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}
	infos, err := svc.ListHarnesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: list harnesses: %w", err)
	}
	var info *agentlib.HarnessInfo
	for i := range infos {
		if infos[i].Name == harnessName {
			info = &infos[i]
			break
		}
	}
	if info == nil {
		return nil, fmt.Errorf("agent: unknown harness: %s", harnessName)
	}

	// Pull binary and reasoning-level metadata from the local registry — the
	// service does not expose these directly today.
	registry := newHarnessRegistry()
	harness, _ := registry.Get(harnessName)

	caps := &HarnessCapabilities{
		Harness:             info.Name,
		Available:           info.Available,
		Binary:              harness.Binary,
		Path:                info.Path,
		Surface:             harness.Surface,
		CostClass:           info.CostClass,
		IsLocal:             info.IsLocal,
		ExactPinSupport:     info.ExactPinSupport,
		SupportsEffort:      len(info.SupportedReasoning) > 0 || harness.EffortFlag != "",
		SupportsPermissions: len(info.SupportedPermissions) > 0 || len(harness.PermissionArgs) > 0,
	}
	if len(info.SupportedReasoning) > 0 {
		caps.ReasoningLevels = append([]string{}, info.SupportedReasoning...)
	} else if len(harness.ReasoningLevels) > 0 {
		caps.ReasoningLevels = append([]string{}, harness.ReasoningLevels...)
	}

	if harness.DefaultModel != "" {
		caps.Model = harness.DefaultModel
		caps.Models = []string{harness.DefaultModel}
	}

	return caps, nil
}

// TestProviderConnectivityViaService runs a HealthCheck against the named
// harness and translates the result into a ProviderStatus. It is the
// production replacement for Runner.TestProviderConnectivity.
func TestProviderConnectivityViaService(ctx context.Context, workDir, harnessName string, timeout time.Duration) ProviderStatus {
	status := ProviderStatus{Reachable: false}
	if ctx == nil {
		ctx = context.Background()
	}
	if harnessName == "virtual" || harnessName == "agent" {
		status.Reachable = true
		status.CreditsOK = true
		return status
	}
	svc, err := NewServiceFromWorkDir(workDir)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if err := svc.HealthCheck(ctx, agentlib.HealthTarget{Type: "harness", Name: harnessName}); err != nil {
		errStr := strings.ToLower(err.Error())
		status.Error = err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "quota") ||
			strings.Contains(errStr, "credit") || strings.Contains(errStr, "insufficient") {
			status.CreditsOK = false
		}
		return status
	}
	status.Reachable = true
	status.CreditsOK = true
	return status
}

// ValidateForExecuteLoopViaService is the production replacement for
// Runner.ValidateForExecuteLoop. When harness is empty it is a no-op (routing
// will pick at claim time). When harness is specified it confirms the harness
// exists in the service registry and (when model is set) attempts a
// ResolveRoute pre-flight.
func ValidateForExecuteLoopViaService(ctx context.Context, workDir, harnessName, model, provider, modelRef string) error {
	if harnessName == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := NewServiceFromWorkDir(workDir)
	if err != nil {
		return fmt.Errorf("agent: build service: %w", err)
	}
	infos, err := svc.ListHarnesses(ctx)
	if err != nil {
		return fmt.Errorf("agent: list harnesses: %w", err)
	}
	found := false
	for _, info := range infos {
		if info.Name == harnessName {
			found = true
			if !info.Available {
				return fmt.Errorf("agent: harness %s not available", harnessName)
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("agent: unknown harness: %s", harnessName)
	}

	// Pre-flight orphan-model check via ResolveRoute. Only meaningful when a
	// model is provided and provider/model-ref are not set.
	if model != "" && provider == "" && modelRef == "" && harnessName == "agent" {
		if _, err := svc.ResolveRoute(ctx, agentlib.RouteRequest{
			Model:    model,
			Harness:  harnessName,
			ModelRef: modelRef,
			Provider: provider,
		}); err != nil {
			return fmt.Errorf("agent: model %q is not routable: %w", model, err)
		}
	}
	return nil
}
