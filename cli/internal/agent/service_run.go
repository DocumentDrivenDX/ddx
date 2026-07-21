package agent

// service_run.go provides Runner-free helpers that drive every operation via
// the agentlib.FizeauService service surface (CONTRACT-003). RunWithConfigViaService
// is the only run entry point; legacy RunViaService/RunViaServiceWith and the
// runFixtureHarnessViaRunner shim were retired in B22d-h.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
)

// serviceRunFactory, when non-nil, overrides NewServiceFromWorkDir inside
// RunWithConfigViaService. CLI-level integration tests inject a stub to
// observe ServiceExecuteRequest fields without a real agent server.
var serviceRunFactory func(workDir string) (agentlib.FizeauService, error)

// SetServiceRunFactory installs a service factory for RunWithConfigViaService.
// Pass nil to restore production behavior. Exported for cmd/ integration tests.
//
// Production reachability: invoked from init() with nil so the symbol is
// reachable from main() under deadcode RTA. Real overrides come from cmd/
// integration tests at test-setup time.
func SetServiceRunFactory(f func(workDir string) (agentlib.FizeauService, error)) {
	serviceRunFactory = f
}

func init() {
	// Reach SetServiceRunFactory from main() so it survives deadcode RTA.
	// The nil reset is a no-op: production never sets the factory.
	SetServiceRunFactory(nil)
}

// ResolveServiceFromWorkDir returns a service instance for the workdir,
// honoring any test override installed via SetServiceRunFactory.
func ResolveServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
	return resolveService(workDir)
}

// ResolvePreflightServiceFromWorkDir returns a short-lived service instance for
// execution and capability queries. Production disables background provider
// probes; tests still honor SetServiceRunFactory so they can observe requests
// without a real service.
func ResolvePreflightServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
	if serviceRunFactory != nil {
		return serviceRunFactory(workDir)
	}
	return NewPreflightServiceFromWorkDir(workDir)
}

// resolveService returns a FizeauService for workDir, using the test-injected
// factory when set and the production NewServiceFromWorkDir otherwise. All
// internal callers must go through this helper so test stubs installed via
// SetServiceRunFactory are honored everywhere.
func resolveService(workDir string) (agentlib.FizeauService, error) {
	factory := serviceRunFactory
	if factory == nil {
		factory = NewServiceFromWorkDir
	}
	return factory(workDir)
}

// RunWithConfigViaService dispatches a single agent invocation through the
// agent service. It takes a sealed ResolvedConfig (durable knobs) and an
// AgentRunRuntime (per-invocation plumbing/intent), and is the SD-024
// successor to the retired RunViaService/RunViaServiceWith entry points.
func RunWithConfigViaService(ctx context.Context, workDir string, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Install the provider-launch PATH shim before constructing the Fizeau
	// service so that when fizeau LookPaths codex/claude/etc it finds our
	// wrapper, which sets PR_SET_PDEATHSIG=SIGKILL before execve'ing the real
	// binary. Without this, provider children survive worker SIGKILL/OOM as
	// ppid=1 orphans (bead ddx-f2b413ea). The shared installer resolves the
	// running ddx executable first and refuses the go test wrapper before any
	// PATH mutation occurs.
	if _, _, shimErr := installProviderShimOnPATH(); shimErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "agent: provider shim install failed (continuing without parent-death protection): %v\n", shimErr)
	}

	svc, err := ResolvePreflightServiceFromWorkDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}
	return executeOnService(ctx, svc, workDir, rcfg, runtime)
}

// executeOnService dispatches against a pre-built svc using the durable
// knobs from rcfg and per-invocation plumbing from runtime, then records
// one session-index row. It is the inlined replacement for the retired
// RunViaServiceWith helper and is shared with dispatchViaResolvedConfig.
func executeOnService(ctx context.Context, svc agentlib.FizeauService, workDir string, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	if svc == nil {
		return nil, fmt.Errorf("agent: nil service")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Ensure the provider-launch PATH shim is in place before dispatching the
	// actual service call. This covers the execute-bead / reviewer dispatch path
	// that reaches executeOnService without going through RunWithConfigViaService.
	if _, _, shimErr := installProviderShimOnPATH(); shimErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "agent: provider shim install failed (continuing without parent-death protection): %v\n", shimErr)
	}

	promptText := runtime.Prompt
	if runtime.PromptFile != "" {
		caps := rcfg.EvidenceCapsForRole(runtime.Role)
		data, err := readPromptFileBoundedWithCap(runtime.PromptFile, caps.MaxPromptBytes)
		if err != nil {
			return nil, fmt.Errorf("agent: read prompt file: %w", err)
		}
		promptText = string(data)
	}
	if err := validateInlinePromptCap(runtime.PromptSource, promptText, rcfg.EvidenceCapsForRole(runtime.Role).MaxPromptBytes); err != nil {
		return nil, fmt.Errorf("agent: prompt for role %q: %w", runtime.Role, err)
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
	if harness == "" && !runtime.ClearRoutingPins {
		harness = pt.Harness
	}

	model := runtime.ModelOverride
	if model == "" && !runtime.ClearRoutingPins {
		model = pt.Model
	}

	provider := runtime.ProviderOverride
	if provider == "" && !runtime.ClearRoutingPins {
		provider = pt.Provider
	}
	if runtime.ClearRoutingPins && runtime.ProviderOverride == "" {
		provider = ""
	}

	profile := runtime.ProfileOverride
	if profile == "" && !runtime.ClearProfile {
		profile = rcfg.Profile()
	}

	permissions := runtime.PermissionsOverride
	if permissions == "" {
		permissions = rcfg.Permissions()
	}

	providerTimeout := rcfg.ProviderRequestTimeout()

	minPower := rcfg.MinPower()
	if runtime.MinPowerOverride > 0 {
		minPower = runtime.MinPowerOverride
	}
	if runtime.ClearMinPower {
		minPower = 0
	}
	maxPower := rcfg.MaxPower()
	if minPower > 0 && maxPower > 0 && minPower >= maxPower {
		return nil, fmt.Errorf("agent: invalid power bounds: min_power=%d must be less than max_power=%d", minPower, maxPower)
	}

	req := agentlib.ServiceExecuteRequest{
		Prompt:                promptText,
		Model:                 model,
		Policy:                profile,
		Provider:              provider,
		Harness:               harness,
		Reasoning:             agentlib.Reasoning(rcfg.Effort()),
		Permissions:           permissions,
		WorkDir:               wd,
		Timeout:               wall,
		IdleTimeout:           idle,
		ProviderTimeout:       providerTimeout,
		SessionLogDir:         sessionLogDir,
		Metadata:              metadataWithEnv(runtime.Correlation, runtime.Env),
		Role:                  runtime.Role,
		CorrelationID:         runtime.CorrelationID,
		MinPower:              minPower,
		MaxPower:              maxPower,
		EstimatedPromptTokens: firstPositiveInt(runtime.EstimatedPromptTokens, estimatePromptTokens(promptText)),
		RequiresTools:         runtime.RequiresTools,
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// When Harness is set and Model is empty, fizeau routes within the harness's
	// eligible models using Profile/MinPower as the routing constraint. This is the
	// well-formed request shape for profile-driven dispatch without an explicit model.
	// fizeau returns ErrHarnessModelIncompatible (typed, non-nil) on pre-dispatch
	// errors and a failed final event for post-dispatch errors.
	start := time.Now().UTC()
	if runtime.OnExecuteStart != nil {
		runtime.OnExecuteStart()
	}
	events, err := svc.Execute(cancelCtx, req)
	if err != nil {
		// A pre-dispatch Execute error means routing never produced a viable
		// dispatch — a provider-boundary failure. Wrap it with its typed
		// classification (ddx-3b721804) so callers recover the taxonomy via
		// errors.As instead of re-parsing free text, and so the failure
		// surfaces as a typed outcome_reason rather than generic execution_failed.
		return nil, &ProviderFailureError{
			Failure: ClassifyServiceExecuteError(err),
			Err:     fmt.Errorf("agent: execute: %w", err),
		}
	}

	workPhase := strings.TrimSpace(runtime.WorkLogPhase)
	if workPhase == "" {
		workPhase = "do"
	}
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		CurrentBeadID: runtime.Correlation["bead_id"],
		WorkPhase:     workPhase,
	})
	watchdog := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     idle,
		toolCallTimeout: time.Duration(ToolCallTimeout) * time.Millisecond,
		// cancelCtx is a child of the caller's ctx, so an external cancel
		// (e.g. execute_bead.go's dispatchCancel, fired by the running-phase
		// guard's harness-liveness watchdog) reaches this select even if the
		// provider keeps emitting events that would otherwise mask the idle
		// timeout (ddx-f2b7cf89).
		ctx: cancelCtx,
	}
	onRouteResolved := func(harness, provider, model string) {
		harness = firstNonEmpty(harness, runtime.HarnessOverride, pt.Harness)
		provider = firstNonEmpty(provider, runtime.ProviderOverride, pt.Provider)
		model = firstNonEmpty(model, runtime.ModelOverride, pt.Model)
		route := providerRouteLabel(provider, model)
		now := time.Now().UTC()
		reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), route, harness, now)
		if len(reaped) > 0 {
			writeProviderChildCleanupArtifact(workDir, runtime.Correlation["attempt_id"], &providerChildCleanupReport{
				AttemptID:   runtime.Correlation["attempt_id"],
				BeadID:      runtime.Correlation["bead_id"],
				Trigger:     reasonSupersededProviderChild,
				ActiveRoute: firstNonEmpty(route, harness),
				ScannedAt:   now,
				Survivors:   survivors,
				Reaped:      reaped,
			})
		}
		if runtime.OnRouteResolved != nil {
			runtime.OnRouteResolved(harness, provider, model)
		}
	}
	final, routing, _ := drainServiceEventsWithRenderer(events, runtime.Output, renderer, watchdog, onRouteResolved)
	if final == nil {
		// Provider process exited before emitting a final event — a harness-level
		// failure (crash, OOM, binary restart) the model never saw. Classify as
		// harness unavailable (retryable) so an unpinned worker can fall back to
		// another eligible route without operator intervention.
		return nil, &ProviderFailureError{
			Failure: ProviderFailure{
				Reason:     FailureModeProviderHarnessUnavailable,
				Retryable:  true,
				Disruption: FailureModeProviderHarnessUnavailable,
			},
			Err: fmt.Errorf("agent: provider process exited without emitting a final event"),
		}
	}
	finishedAt := time.Now().UTC()
	elapsed := finishedAt.Sub(start)

	result := &Result{
		Harness:    harness,
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
		if routing.Reason != "" {
			result.RouteReason = routing.Reason
		}
	}
	var routingActual *agentlib.ServiceRoutingActual
	if final != nil {
		routingActual = final.RoutingActual
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
			if final.Usage.CacheReadTokens != nil {
				result.CachedTokens = *final.Usage.CacheReadTokens
			}
			if final.Usage.TotalTokens != nil {
				result.Tokens = *final.Usage.TotalTokens
			}
		}
		if final.CostUSD != nil && *final.CostUSD > 0 {
			result.CostUSD = *final.CostUSD
		}
		result.ExitCode = final.ExitCode
		result.Error = final.Error
		if final.RoutingActual != nil {
			if final.RoutingActual.Provider != "" {
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
			result.AgentSessionID = final.SessionLogPath
		}
	}
	if candidate, ok := selectedRoutingCandidate(routing, routingActual); ok {
		result.Billing = string(candidate.Billing)
		result.PredictedPower = candidate.Components.Power
		result.PredictedSpeedTPS = candidate.Components.SpeedTPS
		result.PredictedCostUSDPer1kTokens = candidate.CostUSDPer1kTokens
		result.PredictedCostSource = candidate.CostSource
	}
	normalizeServiceFinalExitCode(result)
	entry := SessionIndexEntryFromResult(workDir, SessionIndexInputs{
		Harness:     harness,
		Model:       model,
		Provider:    provider,
		Effort:      rcfg.Effort(),
		Correlation: runtime.Correlation,
	}, result, start, finishedAt)
	_ = AppendSessionIndex(ResolveLogDir(workDir, ""), entry, finishedAt)
	return result, nil
}

func estimatePromptTokens(prompt string) int {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return 0
	}
	return (len(prompt) + 3) / 4
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func metadataWithEnv(metadata, env map[string]string) map[string]string {
	if len(metadata) == 0 && len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(metadata)+len(env))
	for k, v := range metadata {
		out[k] = v
	}
	for k, v := range env {
		out[k] = v
	}
	return out
}

func normalizeServiceFinalExitCode(result *Result) {
	if result == nil {
		return
	}
	if result.ExitCode == 0 && strings.TrimSpace(result.Error) != "" {
		result.ExitCode = 1
	}
}

// CapabilitiesViaService returns the named harness exactly as Fizeau reports
// it. DDx does not augment this inventory with a local harness catalog: route
// availability, defaults, permissions, and reasoning support belong to the
// harness runtime.
func CapabilitiesViaService(ctx context.Context, workDir, harnessName string) (*HarnessCapabilities, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := ResolvePreflightServiceFromWorkDir(workDir)
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

	caps := &HarnessCapabilities{
		Harness:             info.Name,
		Available:           info.Available,
		Path:                info.Path,
		CostClass:           info.CostClass,
		IsLocal:             info.CostClass == "local",
		ExactPinSupport:     info.ExactPinSupport,
		SupportsEffort:      len(info.SupportedReasoning) > 0,
		SupportsPermissions: len(info.SupportedPermissions) > 0,
	}
	caps.ReasoningLevels = append([]string{}, info.SupportedReasoning...)
	if info.DefaultModel != "" {
		caps.Model = info.DefaultModel
		caps.Models = []string{info.DefaultModel}
	}

	return caps, nil
}
