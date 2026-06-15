package agent

// service_run.go provides Runner-free helpers that drive every operation via
// the agentlib.FizeauService service surface (CONTRACT-003). RunWithConfigViaService
// is the only run entry point; legacy RunViaService/RunViaServiceWith and the
// runFixtureHarnessViaRunner shim were retired in B22d-h.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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

// ResolveServiceFromWorkDirCtx returns a context-scoped service instance for
// the workdir, honoring any test override installed via SetServiceRunFactory.
// Production callers with request/attempt lifetimes should prefer this helper
// so Fizeau background probes are cancelled when ctx is cancelled.
func ResolveServiceFromWorkDirCtx(ctx context.Context, workDir string) (agentlib.FizeauService, error) {
	factory := serviceRunFactory
	if factory != nil {
		return factory(workDir)
	}
	return NewServiceFromWorkDirCtx(ctx, workDir)
}

// ResolvePreflightServiceFromWorkDir returns a short-lived service instance for
// pre-dispatch model/route checks. Production uses a constructor with disabled
// background provider probes; tests still honor SetServiceRunFactory so they can
// observe requests without a real service.
func ResolvePreflightServiceFromWorkDir(workDir string) (agentlib.FizeauService, error) {
	if serviceRunFactory != nil {
		return serviceRunFactory(workDir)
	}
	return NewPreflightServiceFromWorkDir(workDir)
}

func cleanupCurrentProcessProviderProbes(ctx context.Context, workDir string) {
	if serviceRunFactory != nil {
		return
	}
	var scopes []string
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		scopes = append(scopes, cwd)
	}
	if workDir = strings.TrimSpace(workDir); workDir != "" {
		scopes = append(scopes, workDir)
	}
	if len(scopes) == 0 {
		return
	}
	cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = ReapRootProviderChildrenInScopes(cctx, os.Getpid(), scopes...)
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
			" or pass --request-timeout DURATION to execute-bead/work",
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

	svc, err := ResolvePreflightServiceFromWorkDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}
	defer cleanupCurrentProcessProviderProbes(ctx, workDir)
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
		Prompt:                promptText,
		Model:                 model,
		Policy:                profile,
		Provider:              provider,
		Harness:               fizeauHarness(harness),
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
	seedRecentRouteAttemptsFromTracker(ctx, svc, workDir, time.Now().UTC())

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
	}
	onRouteResolved := func(harness, provider, model string) {
		harness = firstNonEmpty(harness, fizeauHarness(strings.TrimSpace(runtime.HarnessOverride)), fizeauHarness(strings.TrimSpace(pt.Harness)))
		provider = firstNonEmpty(provider, strings.TrimSpace(runtime.ProviderOverride), strings.TrimSpace(pt.Provider))
		model = firstNonEmpty(model, strings.TrimSpace(runtime.ModelOverride), strings.TrimSpace(pt.Model))
		route := providerRouteLabel(provider, model)
		now := time.Now().UTC()
		reaped, survivors := reapSupersededProviderChildren(context.Background(), os.Getpid(), wd, route, harness, now)
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
		if routing.Reason != "" {
			result.RouteReason = routing.Reason
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
			result.AgentSessionID = final.SessionLogPath
		}
	}
	result.Error = appendProviderTimeoutHint(result.Error, providerTimeout)
	normalizeServiceFinalExitCode(result)
	recordServiceRouteAttempt(ctx, svc, result, elapsed, finishedAt)
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

// knownSubscriptionHarnesses is the fixed set of CLI subscription harnesses.
// It is the fail-open fallback for isShieldedSubscriptionHarness when the live
// harness list cannot prove (or wrongly denies, via tainted liveness) that a
// subscription harness binary is present: a transient connectivity blip must
// never hard-exclude one of these harnesses during a local-fleet outage.
var knownSubscriptionHarnesses = map[string]struct{}{
	"claude":      {},
	"claude-code": {},
	"claude-tui":  {},
	"codex":       {},
	"gemini":      {},
	"gemini-cli":  {},
}

// shieldedSubscriptionHarnesses returns the set of harness names whose recorded
// connectivity failures must NOT be replayed as hard route exclusions. A harness
// is shielded when it is a subscription (flat-rate CLI) harness whose binary is
// discoverable on PATH.
//
// CRITICAL: the dispatchability signal here is binary-on-PATH, NOT the
// HarnessInfo.Available liveness flag. During an outage fizeau's route-health
// store can flip a subscription harness's Available to false purely from the
// replayed connectivity blips we are trying to shield against — using Available
// would make the shield circular (it stops firing exactly when it is needed).
// HarnessInfo.Path is set from exec.LookPath of the harness binary in fizeau's
// registry.Discover() and reflects binary presence, not recent-call success.
//
// Detection is layered, each independent of tainted liveness:
//  1. Live list, billing==subscription AND Path non-empty (binary on PATH).
//  2. Live list, billing==subscription AND name in knownSubscriptionHarnesses
//     (covers a present binary whose Path was momentarily blank).
//  3. Always: every name in knownSubscriptionHarnesses (fail-open when the
//     harness list is unavailable, e.g. svc nil or ListHarnesses errored).
//
// The fixed-set members are always included so that even a hard service error
// cannot let a connectivity blip exclude claude/codex/gemini.
// All keys returned are lower-cased and whitespace-trimmed so callers can match
// with a single normalized membership test (see isShieldedSubscriptionHarness).
func shieldedSubscriptionHarnesses(ctx context.Context, svc agentlib.FizeauService) map[string]struct{} {
	out := map[string]struct{}{}
	for name := range knownSubscriptionHarnesses {
		out[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if svc == nil {
		return out
	}
	infos, err := svc.ListHarnesses(ctx)
	if err != nil {
		return out
	}
	for _, info := range infos {
		if info.Billing != agentlib.BillingModelSubscription {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(info.Name))
		if name == "" {
			continue
		}
		if strings.TrimSpace(info.Path) != "" {
			// Binary discoverable on PATH (registry.Discover sets Path from
			// exec.LookPath, independent of recent-call liveness).
			out[name] = struct{}{}
			continue
		}
		if _, known := knownSubscriptionHarnesses[name]; known {
			out[name] = struct{}{}
		}
	}
	return out
}

// isShieldedSubscriptionHarness reports whether the named route harness/provider
// is a subscription CLI harness whose binary is discoverable on PATH (and thus
// must never be hard-excluded from routing due to replayed connectivity blips).
// shielded is the set returned by shieldedSubscriptionHarnesses; passing it in
// keeps the (potentially I/O-bound) ListHarnesses lookup to one call per
// exclusion pass. The name is matched case/whitespace-insensitively.
func isShieldedSubscriptionHarness(name string, shielded map[string]struct{}) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	_, ok := shielded[n]
	return ok
}

func seedRecentRouteAttemptsFromTracker(ctx context.Context, svc agentlib.FizeauService, projectRoot string, now time.Time) {
	if svc == nil || strings.TrimSpace(projectRoot) == "" {
		return
	}
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	beads, err := store.List("", "", nil)
	if err != nil {
		return
	}
	skipSubscription := shieldedSubscriptionHarnesses(ctx, svc)
	cutoff := now.Add(-ProviderUnavailableCooldown)
	seen := map[string]struct{}{}
	for _, b := range beads {
		for _, failed := range readFailedRoutes(b.Extra) {
			at, _ := time.Parse(time.RFC3339, strings.TrimSpace(failed.At))
			if at.IsZero() {
				at = now
			}
			if at.Before(cutoff) || at.After(now.Add(time.Minute)) || strings.TrimSpace(failed.Provider) == "" {
				continue
			}
			if isShieldedSubscriptionHarness(failed.Provider, skipSubscription) {
				// Shielded subscription harness: a transient connectivity blip
				// must not become a hard exclusion that removes the only live
				// option during a local-fleet outage.
				continue
			}
			key := "\x00" + failed.Provider + "\x00" + failed.Model
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			// Rebase replayed failures to `now` so they survive fizeau's
			// route-health TTL window. Historical tracker timestamps (minutes
			// to ~15 min old) sit outside fizeau's default 30s cooldown and
			// are dropped immediately by ActiveAttempts, leaving policy/default
			// Execute routes free to re-pick the same failed provider. The
			// tracker-side cutoff above (ProviderUnavailableCooldown) decides
			// which failures are still worth replaying; once they pass that
			// gate they are asserted active for fizeau's TTL counted from now.
			_ = svc.RecordRouteAttempt(ctx, agentlib.RouteAttempt{
				Provider:  failed.Provider,
				Model:     failed.Model,
				Status:    "failed",
				Reason:    FailureModeProviderConnectivity,
				Error:     FailureModeProviderConnectivity,
				Timestamp: now,
			})
		}
		events, err := store.Events(b.ID)
		if err != nil {
			continue
		}
		for _, ev := range events {
			if ev.Kind != "route-failure" || ev.CreatedAt.Before(cutoff) || ev.CreatedAt.After(now.Add(time.Minute)) {
				continue
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(ev.Body), &body); err != nil {
				continue
			}
			if strings.TrimSpace(fmt.Sprint(body["outcome_reason"])) != FailureModeProviderConnectivity {
				continue
			}
			provider := strings.TrimSpace(fmt.Sprint(body["provider"]))
			if provider == "" || provider == "<nil>" {
				continue
			}
			harness := strings.TrimSpace(fmt.Sprint(body["harness"]))
			model := strings.TrimSpace(fmt.Sprint(body["model"]))
			if isShieldedSubscriptionHarness(harness, skipSubscription) {
				continue
			}
			if isShieldedSubscriptionHarness(provider, skipSubscription) {
				continue
			}
			key := harness + "\x00" + provider + "\x00" + model
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			_ = svc.RecordRouteAttempt(ctx, agentlib.RouteAttempt{
				Harness:   harness,
				Provider:  provider,
				Model:     model,
				Status:    "failed",
				Reason:    FailureModeProviderConnectivity,
				Error:     strings.TrimSpace(fmt.Sprint(body["error"])),
				Timestamp: now,
			})
		}
	}
}

func recordServiceRouteAttempt(ctx context.Context, svc agentlib.FizeauService, result *Result, elapsed time.Duration, finishedAt time.Time) {
	if svc == nil || result == nil || strings.TrimSpace(result.Provider) == "" {
		return
	}
	status := "success"
	reason := "success"
	if result.ExitCode != 0 || strings.TrimSpace(result.Error) != "" {
		status = "failed"
		reason = "execution_failed"
		if strings.TrimSpace(result.Error) != "" {
			reason = "provider_error"
		}
	}
	_ = svc.RecordRouteAttempt(ctx, agentlib.RouteAttempt{
		Harness:   result.Harness,
		Provider:  result.Provider,
		Model:     result.Model,
		Status:    status,
		Reason:    reason,
		Error:     result.Error,
		Duration:  elapsed,
		Timestamp: finishedAt,
	})
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
	svc, err := ResolvePreflightServiceFromWorkDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent: build service: %w", err)
	}
	defer cleanupCurrentProcessProviderProbes(ctx, workDir)
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
	defer cleanupCurrentProcessProviderProbes(ctx, workDir)
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

// ValidateHarnessOnlyRouteViaService preflights a harness-only request shape
// using the same route resolver the execute path will hit. It is used for
// harness-pinned work/try requests that intentionally leave model empty while
// relying on profile or MinPower to make the route viable.
func ValidateHarnessOnlyRouteViaService(ctx context.Context, workDir, harnessName, provider, profile string, minPower, maxPower int) error {
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
	defer cleanupCurrentProcessProviderProbes(ctx, workDir)
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

	_, err = svc.ResolveRoute(ctx, agentlib.RouteRequest{
		Harness:  fizeauHarness(harnessName),
		Provider: provider,
		Policy:   profile,
		MinPower: minPower,
		MaxPower: maxPower,
	})
	if err != nil {
		return fmt.Errorf("agent: route preflight failed: %w", err)
	}
	return nil
}
