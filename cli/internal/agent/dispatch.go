package agent

// dispatch.go owns the SD-024 dispatch seam shared by execute-bead and the
// post-merge reviewer. It threads durable knobs from a sealed
// config.ResolvedConfig with per-invocation plumbing/intent from an
// AgentRunRuntime through the same service-or-runner routing used by
// RunWithConfigViaService for `ddx agent run`.

import (
	"context"
	"fmt"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// dispatchViaResolvedConfig is the internal SD-024 dispatch seam shared by
// execute-bead's worker and the post-merge reviewer. It resolves how the
// agent invocation is executed (test-injected runner, pre-built service, or
// a fresh service constructed from projectRoot) and assembles a RunOptions
// from rcfg + runtime so every durable knob lands on the dispatched request
// exactly once.
//
// Resolution order:
//  1. runner (test injection seam) — used directly via runner.Run.
//  2. svc (pre-built service) — used via RunViaServiceWith.
//  3. Fallback: construct a fresh service via NewServiceFromWorkDir(projectRoot)
//     and dispatch via RunViaServiceWith.
//
// Override fields on runtime (HarnessOverride, ModelOverride,
// PermissionsOverride, SessionLogDirOverride) take precedence over the
// matching rcfg accessors when non-empty, so callers can pin one knob for
// a single invocation without re-resolving the full ResolvedConfig.
func dispatchViaResolvedConfig(ctx context.Context, projectRoot string, svc agentlib.DdxAgent, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
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

	var opts RunOptions
	opts.Context = ctx
	opts.Harness = harness
	opts.Prompt = runtime.Prompt // evidence:allow-unbounded reason="caller is responsible for bounding the prompt before invoking dispatchViaResolvedConfig; downstream RunViaServiceWith hits readPromptFileBounded for PromptFile inputs"
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

	if runner != nil {
		return runner.Run(opts)
	}
	if svc == nil {
		built, err := NewServiceFromWorkDir(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("agent: build service: %w", err)
		}
		svc = built
	}
	return RunViaServiceWith(ctx, svc, projectRoot, opts)
}
