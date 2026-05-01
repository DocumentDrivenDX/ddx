# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-57388aff`
- Title: execute-loop provider failures on helix machine: OpenRouter guardrail 404 + Alibaba 502
- Labels: ddx, phase:build, kind:correctness, area:agent, area:routing
- Base revision: `07bed7688290498ac491d0501a18a207400c9c1d`
- Execution bundle: `.ddx/executions/20260411T195303-5b6c0922`

## Description
ddx agent execute-loop fails on eitri: agent harness routes to z-ai/glm-5.1 ignoring local vidar config

ddx agent execute-loop consistently fails on the helix machine (eitri) with provider-level errors.
The execute-loop correctly selects and claims beads, but the agent harness never runs the work.
CRITICAL: ddx agent run --harness agent works perfectly on the same machine, pointing to a
model-resolution bug specifically in execute-bead/execute-loop.

PROOF: direct agent run works, execute-loop fails:
  ddx agent run --text "Hello" --harness agent --timeout 60s → succeeds with qwen3.5-27b on vidar
  ddx agent execute-loop --once --json --harness agent → fails with z-ai/glm-5.1 via OpenRouter 404

ROOT CAUSE: Model resolution in execute-bead injects --model z-ai/glm-5.1, overriding the
native ~/.config/agent/config.yaml provider config (default_provider: vidar, model: qwen3.5-27b).
The z-ai/glm-5.1 model is not defined in models.yaml or the native config — it appears to come
from stale execute-bead routing metadata. Even explicit --model flags on execute-loop don't
propagate correctly to the spawned execute-bead subprocess.

PROVIDER STATUS on eitri:
- vidar (http://vidar:1234/v1, qwen3.5-27b): WORKS — confirmed with ddx agent run
- bragi (http://bragi:1234/v1, qwen3.5-27b): DOWN — connection refused
- grendel (http://grendel:1234/v1, qwen3.5-27b): DOWN — connection refused
- openrouter (https://openrouter.ai/api/v1): FAILS — 404 guardrail restriction on claude-haiku

MACHINE CONTEXT (eitri, Linux):
- Hostname: eitri, paths use /home/erik/ and linuxbrew
- ~/.config/agent/config.yaml defines providers with default_provider: vidar
- models.yaml defines code-high profile → agent.openai: gpt-5.4 (OpenRouter surface)
- No ~/.ddx.yml config file
- No project-level .agent/config.yaml
- Codex and claude harnesses nearly out of quota — reserved for review, not bulk execution
- execute-loop was working previously (existing ddx beads show successful runs)
- z-ai/glm-5.1 string appears nowhere in source — comes from runtime routing metadata

ENVIRONMENT VARIABLES: No AGENT_PROVIDER, AGENT_BASE_URL, AGENT_API_KEY, or AGENT_MODEL set.
All routing comes from native ~/.config/agent/config.yaml.

IMPACT: ddx agent execute-loop is the primary execution surface for HELIX beads. If the only
working provider (vidar → qwen3.5-27b) cannot be used from execute-loop, entire queues drain
as failures. The loop records each with retry_after (6 hours) and retries indefinitely.

SUGGESTED FIXES:
1. Fix execute-bead model resolution: respect native .agent/config.yaml provider config or
   properly propagate the --model flag from execute-loop → execute-bead
2. Add provider health checks: ddx agent doctor should test the actual provider with a
   minimal API call, not just check binary existence
3. Fail open on provider errors: when the default provider fails, fall back to the next
   available provider in config rather than hard-failing
4. Guardrail error clarity: surface OpenRouter 404 as "provider unavailable due to
   account restrictions" rather than generic API error
5. Execute-loop resilience: when all providers fail, mark bead truly blocked instead of
   infinite retry_after cycles

FILES INVOLVED:
- cli/internal/agent/agent_runner.go (resolveNativeAgentProvider, resolveAgentConfig, resolveModel)
- cli/cmd/agent_execute_loop.go (execute-loop model flag propagation)
- cli/cmd/agent_execute_bead.go (execute-bead model override logic)
- ~/.config/agent/config.yaml (machine config with vidar provider)

## Acceptance Criteria
No explicit acceptance criteria were recorded on the bead.

## Governing References
No governing references were resolved from the current execution snapshot.

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt.
