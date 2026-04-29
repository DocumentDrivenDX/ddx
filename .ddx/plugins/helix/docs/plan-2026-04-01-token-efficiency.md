# Design Plan: Token Efficiency for helix run

**Date**: 2026-04-01
**Status**: CONVERGED
**Refinement Rounds**: 1

## Problem Statement

`helix run` consumed 18.4M tokens across 54 cycles (340K avg/cycle) in a
17-hour session. At current API pricing this is expensive and unsustainable
for continuous operation. The primary waste is repeated context loading —
each ephemeral cycle re-reads the same project docs, workflow rules, and
codebase structure from scratch.

## Requirements

### Functional

1. Reduce average tokens-per-cycle by at least 40%
2. Preserve correctness — no change to what work gets done or how
3. Preserve the bounded-action contract — each implementation pass is
   still one issue
4. Token usage must be tracked and reported per-cycle and in aggregate

### Non-Functional

- Changes must work with both codex and claude agents
- Must not break deterministic test harness (tests use mock agents)

## Architecture Decisions

### Decision 1: Session reuse vs ephemeral

- **Question**: Should the run loop reuse agent sessions across cycles?
- **Alternatives**:
  - A) Keep ephemeral — simple, no state leakage, but ~100K tokens wasted
    per cycle on context reload
  - B) Persistent session — agent retains context across cycles, much cheaper
    but risk of context pollution between issues
  - C) Session-per-epic — reuse within an epic's children, fresh session
    when switching epics
- **Chosen**: C — session-per-epic
- **Rationale**: Epic focus mode already keeps the loop on one epic's children.
  Those children share the same governing artifacts, codebase area, and context.
  Reusing the session within an epic avoids redundant context loading while
  limiting pollution risk to related work. Fresh session on epic switch.

### Decision 2: Context priming

- **Question**: How to reduce the cost of context loading at session start?
- **Alternatives**:
  - A) No change — agent reads AGENTS.md, workflows, etc. every time
  - B) Pre-computed context file — generate `.helix/context.md` with condensed
    project rules, then pass as the initial prompt
  - C) Smaller AGENTS.md — trim to essentials for implementation
- **Chosen**: B — pre-computed context file
- **Rationale**: AGENTS.md is 300+ lines and growing. The agent only needs a
  subset for implementation. A condensed context file (~50 lines) with the
  project's key rules, current tracker state summary, and quality gate
  commands saves the agent from reading and re-processing the full doc tree.

### Decision 3: Verification scoping

- **Question**: How to reduce verification token cost?
- **Alternatives**:
  - A) Full verification every cycle (current)
  - B) Scoped verification — only check touched crates
  - C) Deferred verification — light check per cycle, full check every N cycles
- **Chosen**: B — scoped verification
- **Rationale**: The agent already knows which files it changed. Running
  `cargo clippy -p <changed-crate>` instead of workspace-wide clippy saves
  significant output tokens. Full verification runs via pre-commit hooks on
  commit, so correctness is preserved.

### Decision 4: Review frequency

- **Question**: Should every cycle get a post-implementation review?
- **Alternatives**:
  - A) Review every cycle (current)
  - B) Review every N cycles
  - C) Review based on change size — skip for small changes
- **Chosen**: C — size-based review
- **Rationale**: A 10-line manifest update doesn't need a fresh-eyes review.
  A 500-line new module does. Threshold: review when diff exceeds 100 lines
  or touches more than 3 files. Always review on epic close (already
  implemented).

### Decision 5: Alignment frequency

- **Question**: How often should periodic alignment run?
- **Alternatives**:
  - A) Every 5 completed cycles (current)
  - B) Every 15 completed cycles
  - C) Only on epic close
- **Chosen**: B — every 15 cycles
- **Rationale**: Alignment reads the full doc tree (~600K tokens). Every 5
  cycles means ~9 alignment runs per 46-cycle session = ~5.4M tokens just on
  alignment. Every 15 cuts that to ~3 runs = ~1.8M tokens. Epic close review
  catches most drift anyway.

## Interface Contracts

### Session reuse (codex)

```bash
# First cycle in epic: start new session (no --ephemeral)
codex exec --progress-cursor -C "$target_root" "$prompt"
# Save session ID from output

# Subsequent cycles in same epic: resume session
codex exec resume --last --progress-cursor -C "$target_root" "$prompt"

# Epic switch: start fresh session
codex exec --progress-cursor -C "$target_root" "$prompt"
```

### Session reuse (claude)

```bash
# First cycle: start session (no --no-session-persistence)
claude -p --permission-mode bypassPermissions --dangerously-skip-permissions "$prompt"

# Subsequent cycles: continue session
claude -p --permission-mode bypassPermissions --dangerously-skip-permissions \
  --continue "$prompt"

# Epic switch: start fresh
claude -p --permission-mode bypassPermissions --dangerously-skip-permissions "$prompt"
```

### Pre-computed context file

Generated at run start and on epic switch:

```
# .helix/context.md
## Project: <name>
## Key Rules
- <condensed from AGENTS.md — quality gates, tracker commands, test commands>
## Current Scope
- Epic: <id> — <title>
- Governing: <spec-id>, <parent artifacts>
## Quality Commands
- <fmt command>
- <clippy command for relevant crates>
- <test command for relevant crates>
```

### Review threshold

New flag: `--review-threshold <lines>` (default: 100)

Skip post-implementation review when `git diff --stat HEAD~1 | tail -1`
shows fewer than threshold lines changed and fewer than 3 files.

## Implementation Plan

### Dependency Graph

```
1. Context priming (.helix/context.md generator)
   |
2. Session reuse in run_agent_prompt
   |
3. Review threshold in run_loop
   |
4. Verification scoping in implementation prompt
   |
5. Alignment frequency default change
   |
6. Tests
```

### Issue Breakdown

1. **Context priming** — Add `generate_run_context()` to scripts/helix that
   writes `.helix/context.md`. Called at run start and epic switch. The
   implementation prompt prepends this file.
   AC: context file generated; implementation prompt references it; tokens
   per first-cycle reduced.

2. **Session reuse** — Modify `run_agent_prompt` for both codex and claude
   to support session persistence. Track session ID. Resume within epic,
   fresh on epic switch. Add `--ephemeral` flag to force old behavior.
   AC: codex cycles within same epic reuse session; fresh session on epic
   switch; tests pass with mock agents (mocks don't support resume, so
   test with --ephemeral).

3. **Review threshold** — Add `--review-threshold` flag. In run_loop,
   check diff size before dispatching review. Always review on epic close.
   AC: small changes skip review; large changes get reviewed; epic close
   always reviewed; test covers threshold logic.

4. **Verification scoping guidance** — Update implementation.md to tell
   the agent: "Run verification only on crates you changed, not the full
   workspace. The pre-commit hooks handle full verification on commit."
   AC: implementation prompt says this; no code change needed.

5. **Alignment frequency** — Change `review_every` default from 5 to 15.
   AC: default changed; tests updated.

6. **Tests** — Update helix-cli.sh for new defaults and threshold behavior.
   AC: all tests pass.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Session reuse causes context pollution between issues | M | M | Fresh session on epic switch; bounded actions still isolated |
| Skipping review misses bugs in small changes | L | M | Pre-commit hooks catch most issues; epic close review catches the rest |
| Codex resume semantics change in future versions | L | M | Fall back to ephemeral if resume fails |
| Context file becomes stale mid-run | L | L | Regenerate on epic switch |

## Expected Impact

| Optimization | Estimated Savings |
|-------------|------------------|
| Session reuse (within epic) | -30% avg tokens/cycle |
| Context priming | -10% first-cycle tokens |
| Review threshold | -15% (skip review on ~50% of small cycles) |
| Verification scoping | -5% (less output to process) |
| Alignment frequency 5→15 | -3.6M tokens per 46-cycle run |
| **Total** | **~40-50% reduction** |

At 340K avg/cycle, a 40% reduction brings it to ~200K/cycle.
A 46-cycle run drops from 18.4M to ~11M tokens.

## Governing Artifacts

- `docs/helix/01-frame/prd.md` — observability requirements, token tracking
- `docs/helix/01-frame/features/FEAT-002-helix-cli.md` — CLI contract
- `workflows/EXECUTION.md` — run loop behavior
- `scripts/helix` — implementation
