# Task Dispatch — Power, Persona, Passthrough

> **Debugging routing?** Routing lives entirely in Fizeau. See `CONTRACT-003-fizeau-service` (in the agent repo, `docs/helix/02-design/contracts/`) for the public surface; routing internals are inside that module.

DDx invokes task execution through the upstream Fizeau service. DDx does not
choose harnesses, providers, endpoints, or models on the normal work path. It
describes the work, forwards raw passthrough constraints for harness,
provider, model, and profile unchanged, requests an abstract power
level, and lets Fizeau route. Fizeau owns concrete routing,
provider/model discovery, alias resolution, fuzzy matching of raw model
strings, transcript/session rendering, and all route-side errors.

## Power bounds

Normal execution should use power bounds:

```bash
ddx run --min-power 10 --prompt task.md
```

Power is an integer scale exposed by the Fizeau contract. DDx passes
`MinPower` and optional `MaxPower` bounds and treats them as ordering
constraints, not as model identities. DDx does not query a model catalog,
discover providers, or pin concrete routes on the normal work path. Fizeau
reports what it actually used:

```text
running with qwen 3.6-27b (power 10)
```

DDx owns bead retry policy. It decides whether an attempt succeeded using DDx
evidence: commits, no-changes rationale, merge/preserve result, gates, review
verdicts, and cooldowns. If a retry is allowed, DDx may raise `MinPower`.

Fizeau owns route selection within those requested bounds. DDx may raise
`MinPower` on retries, but it must not pin a concrete model/provider on the
normal work path or infer route selection from a catalog.

## Passthrough constraints

`--harness`, `--provider`, `--model`, and `--profile` are
allowed only as passthrough constraints:

```bash
ddx run --min-power 10 --model qwen36 --prompt task.md
```

DDx sends these values to Fizeau unchanged. DDx does not validate them,
score them, use them for queue policy, rewrite them on retry, or use them as a
fallback mechanism. Fizeau owns any fuzzy matching, alias resolution, provider
fallback, or typed error for those raw strings. DDx stops on hard-pin
exhaustion; it does not remove pins, widen pins, call route-selection helpers,
or retry in a loop.

## Execution layering

The intended command layering is:

- `ddx run` — one Fizeau `Execute` call with requested `MinPower`/`MaxPower`.
  No route-selection helper.
- `ddx try <bead>` — one bead attempt layered over `run`.
- `ddx work` — queue drain and retry policy layered over `try`.

Route diagnostics are status/debug-only. Do not use diagnostic route decisions
in `run`, `try`, or `work`, and never feed one back into `Execute`.

## Personas

Personas are reusable AI personality templates. DDx injects a
persona's body as a system-prompt addendum to `ddx run` —
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
ddx run --persona code-reviewer --prompt review.md
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

For multi-agent review or adversarial checks, use a workflow skill composition
such as `compare-prompts`, covered in `reference/review.md`:

```bash
ddx run --persona code-reviewer --prompt review.md
ddx run --persona architect --prompt review.md
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

- **Stacking power bounds and passthrough pins casually**. Normal DDx work
  should request `MinPower`. Use passthrough pins only for explicit operator
  constraints; DDx must not interpret them.
- **Hardcoding a harness name in a workflow**. Workflow should
  dispatch by power or role binding; letting each project route through its
  agent configuration is the point of DDx's agent boundary.
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
ddx run --min-power 10 --prompt task.md
ddx run --top-power --prompt task.md
ddx run --min-power 10 --max-power 20 --prompt task.md
ddx run --min-power 10 --provider openrouter --model qwen36 --profile default --prompt task.md
ddx run --persona code-reviewer --prompt review.md

# Introspection
ddx doctor                              # environment health
ddx status                              # project drift / stale docs
ddx bead status                         # queue state
# Fizeau diagnostics, if exposed by the project, remain separate from DDx verbs.

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

Full flag list: `ddx run --help`, `ddx persona --help`,
`ddx install --help`.
