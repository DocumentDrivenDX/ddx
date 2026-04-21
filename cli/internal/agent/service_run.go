package agent

// service_run.go provides Runner-free helpers that drive every operation via
// the agentlib.DdxAgent service surface (CONTRACT-003). Production code
// callers should prefer these helpers over Runner methods so the legacy
// Runner type can be retired when test scaffolding is migrated.
//
// Each helper constructs a fresh service from workDir. When a caller dispatches
// many calls, build the service once with NewServiceFromWorkDir and reuse it.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
)

// RunViaService dispatches a single agent invocation through the agent
// service and returns a populated Result. It is the production replacement
// for Runner.Run.
//
// workDir is the project root used to load provider config; opts mirrors the
// existing Runner RunOptions. SessionLogDir falls back to DefaultLogDir when
// neither opts.SessionLogDir nor an explicit override is provided.
//
// The virtual and script harnesses route through a DDx-owned Runner path,
// not the upstream service. These are not "carve-outs pending migration" —
// they are different products from upstream's same-named stubs. DDx's
// virtual is a content-addressed record/replay dictionary keyed by
// PromptHash; upstream's is a unit-test stub where callers stuff
// virtual.response into Metadata. DDx's script reads a filesystem directive
// file; upstream does not model this at all. See runFixtureHarnessViaRunner.
func RunViaService(ctx context.Context, workDir string, opts RunOptions) (*Result, error) {
	resolvedHarness := opts.Harness
	if resolvedHarness == "" && opts.Provider != "" {
		resolvedHarness = opts.Provider
	}
	if resolvedHarness == "virtual" || resolvedHarness == "script" {
		return runFixtureHarnessViaRunner(ctx, workDir, opts)
	}
	svc, err := NewServiceFromWorkDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}
	return RunViaServiceWith(ctx, svc, workDir, opts)
}

// runFixtureHarnessViaRunner dispatches DDx's fixture-driven harnesses
// (virtual, script) through the local Runner path. These harnesses are
// DDx-owned products — virtual keys responses by a hash of the incoming
// prompt for deterministic record/replay, script runs a directive file
// against the worktree. Neither maps cleanly to the upstream v0.8.0
// service-level virtual/script dispatch (agent-81830379), which takes
// virtual.response or virtual.dict_dir metadata per call rather than a
// keyed dictionary. Callers targeting the upstream stubs should construct
// a direct Service.Execute request with Metadata populated.
func runFixtureHarnessViaRunner(ctx context.Context, workDir string, opts RunOptions) (*Result, error) {
	cfg := Config{SessionLogDir: ResolveLogDir(workDir, "")}
	r := NewRunner(cfg)
	r.WorkDir = workDir
	if opts.Context == nil && ctx != nil {
		opts.Context = ctx
	}
	return r.Run(opts)
}

// RunViaServiceWith is the variant of RunViaService that accepts a pre-built
// DdxAgent so callers issuing many requests can reuse one service instance.
func RunViaServiceWith(ctx context.Context, svc agentlib.DdxAgent, workDir string, opts RunOptions) (*Result, error) {
	if svc == nil {
		return nil, fmt.Errorf("agent: nil service")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Context != nil {
		ctx = opts.Context
	}

	// Resolve prompt either from inline text or PromptFile.
	promptText := opts.Prompt
	if opts.PromptFile != "" {
		data, err := os.ReadFile(opts.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("agent: read prompt file: %w", err)
		}
		promptText = string(data)
	}
	if promptText == "" {
		return nil, fmt.Errorf("agent: prompt is required")
	}

	wd := opts.WorkDir
	if wd == "" {
		wd = workDir
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}

	idle := opts.Timeout
	if idle <= 0 {
		idle = time.Duration(DefaultTimeoutMS) * time.Millisecond
	}
	wall := opts.WallClock
	if wall <= 0 {
		wall = time.Duration(DefaultWallClockMS) * time.Millisecond
	}

	logDir := opts.SessionLogDir
	if logDir == "" {
		logDir = ResolveLogDir(workDir, "")
	}

	harness := opts.Harness
	if harness == "" && opts.Provider != "" {
		// Honour --provider as a harness alias when no harness is explicit
		// (matches Runner.resolveHarness behaviour).
		harness = opts.Provider
	}

	req := agentlib.ServiceExecuteRequest{
		Prompt:          promptText,
		Model:           opts.Model,
		Provider:        opts.Provider,
		Harness:         harness,
		ModelRef:        opts.ModelRef,
		Reasoning:       agentlib.Reasoning(opts.Effort),
		Permissions:     opts.Permissions,
		WorkDir:         wd,
		Timeout:         wall,
		IdleTimeout:     idle,
		ProviderTimeout: DefaultProviderRequestTimeout,
		SessionLogDir:   logDir,
		Metadata:        opts.Correlation,
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	start := time.Now()
	events, err := svc.Execute(cancelCtx, req)
	if err != nil {
		return nil, fmt.Errorf("agent: execute: %w", err)
	}

	final, toolCalls, routing := drainServiceEvents(events)
	elapsed := time.Since(start)

	result := &Result{
		Harness:    harness,
		Model:      opts.Model,
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
		if final.SessionLogPath != "" {
			result.AgentSessionID = final.SessionLogPath
		}
	}
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

	// Profile mappings come from the local model catalog because the service
	// does not surface a per-tier resolution endpoint today.
	if harness.Surface != "" {
		cat := BuiltinCatalog
		for _, profile := range []string{"cheap", "fast", "smart"} {
			if m, ok := cat.Resolve(profile, harness.Surface); ok {
				if caps.ProfileMappings == nil {
					caps.ProfileMappings = make(map[string]string)
				}
				caps.ProfileMappings[profile] = m
			}
		}
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
