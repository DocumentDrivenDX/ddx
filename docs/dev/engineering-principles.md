# Engineering Principles

> For user-facing domain principles, see [docs/helix/01-frame/principles.md](../helix/01-frame/principles.md).

These are the internal engineering principles that shape how DDx is built. They guide architectural decisions, feature scoping, and tradeoffs across the codebase.

### 1. Platform Not Methodology

- **Rule:** DDx provides primitives; workflow tools provide opinions. The CLI owns document library, bead tracker, task execution boundaries, personas, templates, and git sync. Phase enforcement, supervisory loops, and methodology-specific validation belong in workflow tools (HELIX, etc.).
- **Decision generated:** Bead state machine is minimal (open/ready/blocked/closed) with no phase-gate semantics. HELIX layers its frame/design/test/build phases on top via labels and queries.
- **Alternative rejected:** Bake HELIX phases into `ddx bead` so phase transitions are first-class. Rejected because it would couple DDx to one methodology and exclude other workflow tools from the platform.
- **Tradeoff:** Workflow tools must coordinate state across DDx primitives, which adds integration work. In exchange, DDx stays methodology-agnostic and reusable.
- **DDx feature it shapes:** `ddx bead` — a generic work-item store with a dependency DAG, no opinions about what "ready" means beyond unblocked dependencies.

### 2. Project-Local by Default

- **Rule:** Resources install under the project's `.ddx/`, `.agents/`, and `.claude/` directories. There is no home-directory install path and no `--global` surface.
- **Decision generated:** `ddx install <name>` writes only under `<projectRoot>/.ddx/plugins/`, `<projectRoot>/.agents/skills/`, and `<projectRoot>/.claude/skills/`. The default plugin from `library/` is materialized to `.ddx/plugins/ddx/` by `ddx init`.
- **Alternative rejected:** Support a global `~/.ddx/` install for shared resources across projects, mirroring tools like `npm -g` or `cargo install`. Rejected because global state desyncs from per-project version pins and makes reproducible builds harder.
- **Tradeoff:** Users pay disk and install time per project rather than once per machine. In exchange, every project carries its own pinned, reproducible toolkit and there is no "works on my machine" caused by a stale global install.
- **DDx feature it shapes:** `ddx init` and `ddx install` — both write only into the project tree; the retired `--global` flag and `~/.ddx` paths are gone.

### 3. Bounded Context per Attempt

- **Rule:** Each agent attempt runs against a fresh, scoped context — one bead, one worktree, one prompt. Long-running agent state does not leak between attempts.
- **Decision generated:** `ddx try` checks out an isolated git worktree at a specific base revision, runs one bounded Fizeau `Execute` call with only the bead description and governing artifacts, then merges or preserves the result. Failed attempts re-queue with a fresh worktree, not a resumed session.
- **Alternative rejected:** Keep a persistent agent session per bead that retains memory across retries, so the agent "learns" from prior attempts. Rejected because accumulated context drifts toward confusion and makes failures harder to reproduce.
- **Tradeoff:** Each retry repays the cost of re-reading the bead and codebase. In exchange, every attempt is reproducible, debuggable, and unaffected by prior failed reasoning.
- **DDx feature it shapes:** `ddx try` and `ddx work` (the queue drainer) — both create a new worktree per attempt and pass review findings forward only as explicit `<review-findings>` prompt sections, not as session state.

### 4. Evidence on Disk

- **Rule:** Every execution leaves a durable trail under `.ddx/executions/` — prompts, transcripts, diffs, and rationale — so a human or downstream agent can audit what happened without re-running anything.
- **Decision generated:** Each `ddx try` attempt writes an execution bundle (timestamp + short hash) containing the prompt, agent transcript, and either the merge result or a `no_changes_rationale.txt`. Bundles are committed.
- **Alternative rejected:** Stream execution telemetry to an external service (or stdout only) and treat the worktree as ephemeral. Rejected because it makes post-hoc review depend on external availability and prevents agents from reading their own prior attempts.
- **Tradeoff:** Repository size grows with execution count, and bundles must be pruned periodically. In exchange, every bead has reviewable provenance and review agents can read prior-attempt evidence directly from the repo.
- **DDx feature it shapes:** `.ddx/executions/<timestamp>-<hash>/` bundles produced by `ddx try`, including the `no_changes_rationale.txt` convention for intentional non-commits.

### 5. Cheapest Viable, Escalate on Evidence

- **Rule:** Start every attempt with the cheapest and fastest available profile that DDx reasonably expects can complete the task. Escalate only when review, runtime, or retry evidence shows the lower-power path cannot meet the acceptance criteria.
- **Decision generated:** `ddx work` drains the bead queue by introspecting Fizeau's available profiles and model metadata, selecting request-level profile intent without hard-coding profile names, and forwarding raw passthrough constraints unchanged. When a review finds blocking issues, the bead reopens and the next attempt may request a stronger profile or higher `MinPower` with the prior review findings threaded in; Fizeau still chooses the concrete route.
- **Alternative rejected:** Always run the strongest available model for correctness. Rejected because it burns budget on beads a cheap model would have closed cleanly and obscures which work actually needs heavy reasoning.
- **Tradeoff:** Some beads incur a retry round-trip when the cheaper attempt fails, adding latency. In exchange, total spend stays bounded and the system surfaces which beads genuinely require strong models.
- **DDx feature it shapes:** The `ddx work` queue drainer with its review-gated escalation path, paired with Fizeau-owned route selection inside DDx-supplied profile/power intent.

### 6. Reversible Over Ergonomic

- **Rule:** Prefer operations whose effects can be undone with standard tools (git, file deletion) over operations that are smoother but harder to roll back.
- **Decision generated:** `ddx try` produces a merge commit (or preserved branch ref) rather than rebasing or squashing. Failed iterations are preserved as refs, not discarded. Worktrees are isolated so a bad run touches nothing in the user's main checkout.
- **Alternative rejected:** Squash or filter execute-bead history to keep `git log` clean, and auto-discard timed-out iteration refs. Rejected because it destroys evidence and forecloses on manual conflict resolution when an agent's work is salvageable.
- **Tradeoff:** Git history carries more noise (per-attempt commits, preserved refs) and users must occasionally garbage-collect by hand. In exchange, no agent action is unrecoverable and conflict resolution always has the original commits to refer back to.
- **DDx feature it shapes:** The execute-bead merge policy — plain fast-forward or `--no-ff` merge commit only, never squash/rebase/filter — and the preservation of timed-out iteration branches as named refs for manual triage.
