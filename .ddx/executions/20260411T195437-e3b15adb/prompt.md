# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-c1822c89`
- Title: execute-bead routes to z-ai/glm-5.1 ignoring vidar config and models.yaml
- Labels: ddx, phase:build, kind:correctness, area:agent, area:routing
- Base revision: `5936b5ac2c8ecf8c75a802eba044c5355d943ce6`
- Execution bundle: `.ddx/executions/20260411T195437-e3b15adb`

## Description
<ddx agent execute-loop injects z-ai/glm-5.1 model override that fails, while ddx agent run --harness agent works fine with qwen3.5-27b via vidar LMStudio host.>

ddx agent execute-loop on eitri consistently fails because the execute-loop → execute-bead path injects --model z-ai/glm-5.1, overriding both the ~/.config/agent/config.yaml default_provider (vidar → qwen3.5-27b) AND the models.yaml catalog. The same agent harness works perfectly with direct ddx agent run.

PROOF:

1. dd agent run --text "Hello" --harness agent --timeout 60s succeeds:
   - model: qwen3.5-27b
   - exit_code: 0
   - duration_ms: ~44s

2. ddx agent execute-loop --once --json --harness agent fails:
   - ps shows spawned process with --model z-ai/glm-5.1
   - result: execution_failed, 502 Bad Gateway

3. ddx agent execute-loop --once --json --harness agent --model qwen3.5-27b also fails:
   - Still gets a 404 from OpenRouter or 502 from Alibaba
   - execute-loop's --model flag doesn't propagate to the spawned execute-bead subprocess the way expected

ROOT CAUSE: execute-bead resolves the model from ddx metadata/routing (profiles, targets, or cached model catalog) rather than from ~/.config/agent/config.yaml or from execute-loop's --model flag. The z-ai/glm-5.1 model appears to come from a stale or incorrect routing data source.

EVIDENCE:
- models.yaml defines code-high profile with agent.openai: gpt-5.4
- ~/.config/agent/config.yaml defines vidar with qwen3.5-27b
- Neither source mentions z-ai/glm-5.1
- The model appears to come from a separate routing layer in execute-bead or execute-loop (possibly a cached catalog entry or the server's worker routing config)

FILES:
- cli/cmd/agent_execute_loop.go (execute-loop model resolution)
- cli/cmd/agent_execute_bead.go (execute-bead model resolution)  
- cli/internal/agent/agent_runner.go (resolveModel chain)

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
