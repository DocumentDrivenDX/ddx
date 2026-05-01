package agent

// dispatch.go owns the SD-024 dispatch seam shared by execute-bead and the
// post-merge reviewer. It threads durable knobs from a sealed
// config.ResolvedConfig with per-invocation plumbing/intent from an
// AgentRunRuntime through the same service-or-runner routing used by
// RunWithConfigViaService for `ddx agent run`.

import (
	"context"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// dispatchViaResolvedConfig is the internal SD-024 dispatch seam shared by
// execute-bead's worker and the post-merge reviewer. It resolves how the
// agent invocation is executed (test-injected runner, pre-built service, or
// a fresh service constructed from projectRoot) and routes through
// executeOnService so every durable knob lands on the dispatched request
// exactly once.
//
// Resolution order:
//  1. runner (test injection seam) — used directly via runner.Run after
//     applying any AgentRunRuntime overrides.
//  2. svc (pre-built service) — used via executeOnService.
//  3. Fallback: construct a fresh service via NewServiceFromWorkDir(projectRoot)
//     and dispatch via executeOnService.
//
// Override fields on runtime (HarnessOverride, ModelOverride,
// PermissionsOverride, SessionLogDirOverride) take precedence over the
// matching rcfg accessors when non-empty, so callers can pin one knob for
// a single invocation without re-resolving the full ResolvedConfig.
func dispatchViaResolvedConfig(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	if runner != nil {
		return runner.Run(buildRunArgsFromConfig(ctx, rcfg, runtime))
	}
	// DDx-local harnesses (virtual, script) are not upstream service harnesses.
	// Route them through the local Runner so DDX_VIRTUAL_RESPONSES and script
	// directives work correctly via compare and quorum paths, not just agent run.
	harness := runtime.HarnessOverride
	if harness == "" {
		harness = rcfg.Harness()
	}
	if harness == "virtual" || harness == "script" {
		cfg := Config{SessionLogDir: ResolveLogDir(projectRoot, "")}
		r := NewRunner(cfg)
		r.WorkDir = projectRoot
		return r.Run(buildRunArgsFromConfig(ctx, rcfg, runtime))
	}
	if svc == nil {
		factory := serviceRunFactory
		if factory == nil {
			factory = NewServiceFromWorkDir
		}
		built, err := factory(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("agent: build service: %w", err)
		}
		svc = built
	}
	return executeOnService(ctx, svc, projectRoot, rcfg, runtime)
}

// buildRunArgsFromConfig assembles a RunArgs value for the test
// injection runner path. It applies AgentRunRuntime overrides so a caller
// (execute-bead worker, post-merge reviewer) can pin one durable knob for
// a single invocation without re-resolving the full ResolvedConfig.
func buildRunArgsFromConfig(ctx context.Context, rcfg config.ResolvedConfig, runtime AgentRunRuntime) RunArgs {
	harness := runtime.HarnessOverride
	if harness == "" {
		harness = rcfg.Harness()
	}
	model := runtime.ModelOverride
	if model == "" {
		model = rcfg.Model()
	}
	permissions := runtime.PermissionsOverride
	if permissions == "" {
		permissions = rcfg.Permissions()
	}
	sessionLogDir := runtime.SessionLogDirOverride
	if sessionLogDir == "" {
		sessionLogDir = rcfg.SessionLogDir()
	}

	var opts RunArgs
	opts.Context = ctx
	opts.Harness = harness
	// evidence:allow-unbounded reason="caller is responsible for bounding the prompt before invoking dispatchViaResolvedConfig; downstream executeOnService hits readPromptFileBounded for PromptFile inputs"
	opts.Prompt = runtime.Prompt
	opts.PromptFile = runtime.PromptFile
	opts.PromptSource = runtime.PromptSource
	opts.Correlation = runtime.Correlation
	opts.Model = model
	opts.Provider = rcfg.Provider()
	opts.ModelRef = rcfg.ModelRef()
	opts.Effort = rcfg.Effort()
	opts.Timeout = rcfg.Timeout()
	opts.WallClock = rcfg.WallClock()
	opts.WorkDir = runtime.WorkDir
	opts.Permissions = permissions
	opts.SessionLogDir = sessionLogDir
	return opts
}
