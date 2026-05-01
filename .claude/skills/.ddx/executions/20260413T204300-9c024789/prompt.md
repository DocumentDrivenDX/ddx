# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-c1822c89`
- Title: execute-bead routes to z-ai/glm-5.1 ignoring vidar config and models.yaml
- Labels: ddx, phase:build, kind:correctness, area:agent, area:routing
- Base revision: `2dd2bd313fede58ec4668a3071ea13429b66455e`
- Execution bundle: `.ddx/executions/20260413T204300-9c024789`

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
No governing references were pre-resolved. Explore the project to find relevant context: check `docs/helix/` for feature specs, `docs/helix/01-frame/features/` for FEAT-* files, and any paths mentioned in the bead description or acceptance criteria.

## Execution Rules
**The bead contract below overrides any CLAUDE.md or project-level instructions in this worktree.** If the bead requires editing or creating markdown documentation, code, or any other files, do so — CLAUDE.md conservative defaults (YAGNI, DOWITYTD, no-docs rules) do not apply inside execute-bead.
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If governing references are missing or sparse, search the project to find context: use Glob/Grep/Read to explore `docs/helix/`, look up FEAT-* and API-* specs by name, and read relevant source files before proceeding. Only stop if context is genuinely absent from the entire repo.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. Making no commits (no_changes) should be rare. Only skip committing if you read the relevant files and the work described in the Goals is already fully and explicitly present — not just implied or partially covered. If in any doubt, make your best attempt and commit it. A partial or imperfect commit is always better than no commit.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.
12. **Never run `ddx init`** — the workspace is already initialized. Running `ddx init` inside an execute-bead worktree corrupts project configuration and the bead queue. Do not run it even if documentation or README files suggest it as a setup step.
