# Agents — Harness, Profile, Persona Dispatch

> **Debugging routing?** Routing lives entirely in ddx-agent. See `CONTRACT-003-ddx-agent-service` (in the agent repo, `docs/helix/02-design/contracts/`) for the public surface; routing internals are inside that module.

`ddx agent run` is the unified interface for invoking an AI coding
agent through any of DDx's harnesses. Routing decisions (which
harness, which model, which effort level) should flow from
**intent**, not from hand-picking implementation details.

## Profile-first routing

Default to profile:

```bash
ddx agent run --profile default --prompt task.md
```

Each profile names a **tier escalation ladder**. Routing tries the first
tier; if it fails with an escalation-triggering outcome, it retries at
the next tier. The four shipped profiles:

- `cheap` — **local only**. Ladder: `[cheap]`. Never escalates. High-
  volume exploratory work where cost dominates and adequate quality
  suffices.
- `default` — **local first, escalate on failure**. Ladder:
  `[cheap, standard, smart]`. The common case. Tries the local model
  first; escalates to cloud only when the local path genuinely fails
  (execution_failed, land_conflict, post_run_check_failed,
  structural_validation_failed). Throughput-per-dollar optimized.
- `fast` — **cloud-fast, skip local**. Ladder: `[fast, smart]`. For
  latency-critical or interactive work where the local model's 9s
  reasoning warmup is unacceptable. Escalates fast→smart on failure.
- `smart` — **cloud high-quality, no escalation**. Ladder: `[smart]`.
  For bead review, adversarial checks, beads whose AC demands high-
  quality reasoning. If smart fails, the work fails meaningfully — no
  silent fallback.

Profiles are the portable intent signal. DDx resolves each ladder tier's
model ref to a concrete (harness, model, effort) tuple per the project's
`.ddx/config.yaml` `routing.profile_ladders` + `model_overrides` and the
shared catalog. See FEAT-006 §"Profile Semantics" for the full contract.

## Explicit overrides

Override routing only when you have a reason:

```bash
ddx agent run --harness codex --prompt task.md    # force a harness
ddx agent run --model qwen3.6 --prompt task.md    # exact model pin
ddx agent run --effort high --prompt task.md      # effort override
```

Don't stack `--profile` and `--model` together unintentionally —
`--model` is an override that supersedes the profile. If you want
"smart profile but pinned to this model", that's the model; drop
the profile flag.

**Reasons to override:**

- Reproducing a bug specific to one harness or model.
- Controlled A/B tests between harnesses.
- Provider-specific features (e.g., Claude extended thinking).
- Cost/quota management on a specific provider.

**Reasons NOT to override:**

- "This harness is my favorite." Use the profile and let routing
  decide.
- "I don't trust the routing." File a bug; don't work around it.

## Personas

Personas are reusable AI personality templates. DDx injects a
persona's body as a system-prompt addendum to `ddx agent run` —
same persona, same behavior across every harness, because the
harness sees a flat system prompt, not a persona file.

### Default roster

The default `ddx` plugin ships five role-focused personas:

- `code-reviewer` — security-first review with structured verdicts
- `test-engineer` — stubs-over-mocks, real-e2e, baselined
  performance, testing pyramid as shape not ratios
- `implementer` — YAGNI / KISS / DOWITYTD, ships tests with code
- `architect` — opinionated on when to reach for each pattern
  (monolith-first, data-model-first)
- `specification-enforcer` — refuses drift from governing artifacts

See `.ddx/plugins/ddx/personas/README.md` for the quality bar and
authoring guidance. Projects can install additional personas via
plugins.

### Using a persona

Directly at invocation time:

```bash
ddx agent run --persona code-reviewer --prompt review.md
```

Or by binding a role in `.ddx/config.yaml`:

```yaml
persona_bindings:
  code-reviewer: code-reviewer
  test-engineer: test-engineer
  architect: architect
```

Workflows then reference the role abstractly (e.g., "dispatch to
the `code-reviewer` role"), and DDx resolves the binding at
dispatch time.

```bash
ddx persona list                # available personas
ddx persona show <name>         # persona body + frontmatter
ddx persona bind <role> <name>  # set a binding
ddx persona bindings            # current role → persona map
```

## Quorum

For multi-harness review or adversarial checks, use quorum dispatch
— covered in `reference/review.md`:

```bash
ddx agent run --quorum=majority --harnesses=claude,codex \
  --prompt review.md
```

## Getting more capabilities

`ddx install <name>` adds plugins to the project. Plugins can ship
personas, prompts, patterns, templates, and workflow skills.

```bash
ddx install helix              # HELIX workflow plugin
ddx search <term>              # discover available plugins
ddx installed                  # list installed plugins
ddx uninstall <name>           # remove
```

Custom personas go in `.ddx/plugins/<plugin>/personas/<name>.md`
(or directly in `library/personas/<name>.md` for local-only use).
See the personas README for the authoring quality bar.

## Anti-patterns

- **Stacking `--profile` and `--model` / `--effort`**. If `--model`
  is set, `--profile` is moot. Pick one intent signal.
- **Hardcoding a harness name in a workflow**. Workflow should
  dispatch by profile or role binding; letting each project choose
  its harness is the point of DDx's routing layer.
- **Persona for everything**. Personas add a system-prompt addendum;
  they don't make a bad prompt good. Start with a clear prompt;
  reach for a persona when you want consistent style/standards
  across invocations.
- **Persona files in skill directories**. Personas live in
  `library/personas/` or `.ddx/plugins/<plugin>/personas/`, not in
  `.claude/skills/` or `.agents/skills/`. Don't mix the two.

## CLI reference

```bash
# Dispatch
ddx agent run --profile smart --prompt task.md
ddx agent run --harness claude --prompt task.md
ddx agent run --persona code-reviewer --prompt review.md
ddx agent run --quorum=majority --harnesses=claude,codex --prompt p.md
ddx agent run --automation=plan|auto|yolo

# Introspection
ddx agent list                         # available harnesses
ddx agent capabilities <harness>       # model + effort options
ddx agent doctor                       # harness health
ddx agent log                          # recent invocation metadata
ddx agent log <session-id>             # one invocation's detail

# Personas
ddx persona list
ddx persona show <name>
ddx persona bind <role> <persona>
ddx persona bindings

# Plugin install
ddx install <plugin>
ddx search <term>
ddx installed
ddx uninstall <name>
```

Full flag list: `ddx agent run --help`, `ddx persona --help`,
`ddx install --help`.
