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

// fizeauHarness maps DDx internal harness names to the Fizeau wire contract.
// "agent" is DDx's internal name for the embedded native harness; Fizeau
// v0.10.12+ uses "fiz" as the native harness identity on its wire API.
func fizeauHarness(name string) string {
	if name == "agent" {
		return "fiz"
	}
	return name
}

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

// resolveService returns a FizeauService for workDir, using the test-injected
// factory when set and the production NewServiceFromWorkDir otherwise. All
// internal callers (RunWithConfigViaService, ValidateForExecuteLoopViaService,
// etc.) must go through this helper so test stubs installed via
// SetServiceRunFactory are honored everywhere.
func resolveService(workDir string) (agentlib.FizeauService, error) {
	factory := serviceRunFactory
	if factory == nil {
		factory = NewServiceFromWorkDir
	}
	return factory(workDir)
}

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

	effectivePerms := runtime.PermissionsOverride
	if effectivePerms == "" {
		effectivePerms = rcfg.Permissions()
	}
	if err := ValidateReadOnlyReviewerDispatch(harness, effectivePerms); err != nil {
		return nil, err
	}

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
			Effort:        rcfg.Effort(),
			Timeout:       rcfg.Timeout(),
			WallClock:     rcfg.WallClock(),
			WorkDir:       runtime.WorkDir,
			Permissions:   rcfg.Permissions(),
			SessionLogDir: sessionLogDir,
			Env:           runtime.Env,
		})
	}

	svc, err := resolveService(workDir)
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

	providerTimeout := ResolveProviderRequestTimeout(workDir, provider, model, rcfg.ProviderRequestTimeout())

	minPower := rcfg.MinPower()
	if runtime.MinPowerOverride > 0 {
		minPower = runtime.MinPowerOverride
	}
	if runtime.ClearMinPower {
		minPower = 0
	}
	maxPower := rcfg.MaxPower()
	if runtime.ClearMaxPower {
		maxPower = 0
	}
	if minPower > 0 && maxPower > 0 && minPower >= maxPower {
		return nil, fmt.Errorf("agent: invalid power bounds: min_power=%d must be less than max_power=%d", minPower, maxPower)
	}

	req := agentlib.ServiceExecuteRequest{
		Prompt:          promptText,
		Model:           model,
		Policy:          profile,
		Provider:        provider,
		Harness:         fizeauHarness(harness),
		Reasoning:       agentlib.Reasoning(rcfg.Effort()),
		Permissions:     permissions,
		WorkDir:         wd,
		Timeout:         wall,
		IdleTimeout:     idle,
		ProviderTimeout: providerTimeout,
		SessionLogDir:   sessionLogDir,
		Metadata:        metadataWithEnv(runtime.Correlation, runtime.Env),
		Role:            runtime.Role,
		CorrelationID:   runtime.CorrelationID,
		MinPower:        minPower,
		MaxPower:        maxPower,
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// When Harness is set and Model is empty, fizeau routes within the harness's
	// eligible models using Profile/MinPower as the routing constraint. This is the
	// well-formed request shape for profile-driven dispatch without an explicit model.
	// fizeau returns ErrHarnessModelIncompatible (typed, non-nil) on pre-dispatch
	// errors and a failed final event for post-dispatch errors.
	start := time.Now().UTC()
	events, err := svc.Execute(cancelCtx, req)
	if err != nil {
		return nil, fmt.Errorf("agent: execute: %w", err)
	}

	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		CurrentBeadID: runtime.Correlation["bead_id"],
		WorkPhase:     "do",
	})
	watchdog := &drainWatchdog{
		cancel:          cancel,
		idleTimeout:     idle,
		toolCallTimeout: time.Duration(ToolCallTimeout) * time.Millisecond,
	}
	final, routing, _ := drainServiceEventsWithRenderer(events, runtime.Output, renderer, watchdog)
	finishedAt := time.Now().UTC()
	elapsed := finishedAt.Sub(start)

	result := &Result{
		Harness:    fizeauHarness(harness),
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
			result.AgentSessionID = final.SessionLogPath
		}
	}
	result.Error = appendProviderTimeoutHint(result.Error, providerTimeout)
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

// CapabilitiesViaService returns HarnessCapabilities for the named harness by
// querying the service's ListHarnesses and (best-effort) the harness registry.
// It is the production replacement for Runner.Capabilities.
func CapabilitiesViaService(ctx context.Context, workDir, harnessName string) (*HarnessCapabilities, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := resolveService(workDir)
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
		IsLocal:             info.CostClass == "local",
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
	svc, err := resolveService(workDir)
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
func ValidateForExecuteLoopViaService(ctx context.Context, workDir, harnessName, model, provider string) error {
	if harnessName == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := resolveService(workDir)
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
	// model is provided and provider is not set.
	if model != "" && provider == "" && harnessName == "agent" {
		if _, err := svc.ResolveRoute(ctx, agentlib.RouteRequest{
			Model:    model,
			Harness:  fizeauHarness(harnessName),
			Provider: provider,
		}); err != nil {
			return fmt.Errorf("agent: model %q is not routable: %w", model, err)
		}
	}
	return nil
}

// ValidateEffortForRunViaService rejects effort requests that no currently
// available harness can satisfy. `ddx agent run` passes effort through to the
// service rather than resolving a route locally, but the command still needs a
// fast failure for obviously impossible combinations so operators get a useful
// error instead of a silent success path.
func ValidateEffortForRunViaService(ctx context.Context, workDir, profile, effort string) error {
	if effort == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := resolveService(workDir)
	if err != nil {
		return fmt.Errorf("agent: build service: %w", err)
	}
	infos, err := svc.ListHarnesses(ctx)
	if err != nil {
		return fmt.Errorf("agent: list harnesses: %w", err)
	}
	registry := newHarnessRegistry()
	for _, info := range infos {
		if !info.Available {
			continue
		}
		if harness, ok := registry.Get(info.Name); ok && harness.EffortFlag != "" {
			return nil
		}
	}
	if profile == "" {
		return fmt.Errorf("agent: no selected provider candidate satisfies effort %q", effort)
	}
	return fmt.Errorf("agent: no selected provider candidate satisfies profile %q and effort %q", profile, effort)
}
