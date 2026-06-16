---
title: Getting Started
weight: 1
prev: /docs
next: /docs/concepts
---

Get DDx installed and run the first pass through the DDx operator loop:
plan, execute, measure, adapt.

{{< asciinema src="07-quickstart" cols="100" rows="30" >}}

## Install

Run the install script to set up DDx globally:

```bash
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
```

This installs the `ddx` CLI binary to `~/.local/bin/ddx`. Project skills
and workflow plugins are resolved per project by `ddx init` and
`ddx plugin install <plugin>`; payloads live in the XDG plugin cache and
generated agent adapters stay out of git.

Verify the installation:

```bash
ddx version
ddx doctor
```

## Initialize a Project

In your project directory, run:

```bash
ddx init .
```

This creates:
- `.ddx/config.yaml` and `.ddx/versions.yaml` - project DDx metadata
- `.agents/skills/ddx` and `.claude/skills/ddx` - generated adapters to the
  built-in DDx skill package
- `.gitignore` rules that keep generated adapters and plugin payloads out of git

## Install HELIX Workflow

```bash
ddx plugin install helix
```

This records HELIX in `.ddx/plugins.lock.yaml`, resolves the HELIX payload into
`${XDG_DATA_HOME}/ddx/cache/plugins/helix/<version>/`, and generates local
agent adapters under `.agents/skills/` and `.claude/skills/`.

## Plan

Start by writing down what should change. In a small project that might be a
short spec, a README update, or a bead description. In a HELIX project it may
be a PRD, feature spec, design, or test plan.

The planning output should name:

- the intended behavior
- the acceptance criteria
- the measurement or test that proves the work is done
- any scope boundaries the agent must respect

## Track Work

```bash
ddx bead create "Build login page" --type task
ddx bead create "Add auth middleware" --type task
ddx bead list
ddx bead ready
```

Beads are the executable form of the plan. They carry the contract that agents
or humans will work against.

## Execute

Once you have at least one provider configured in your agent service
([fizeau](https://github.com/DocumentDrivenDX/fizeau)), draining the bead
queue requires no per-project configuration:

```bash
ddx work --once
```

DDx delegates provider resolution to the agent service and selects a
cheap-tier model by default — no `.ddx/config.yaml` is required, and no
`--harness`, `--profile`, `--model`, or `--provider` flags are needed to
start. Provider configuration and any "no providers configured" errors
are reported by the agent service.

## Measure

After a run, inspect the evidence before trusting the result:

```bash
ddx bead show <id>
ddx bead review <id>
ddx bead metrics <id>
ddx doc stale
```

Run the bead's acceptance commands yourself when closing important work. DDx
records run evidence, review output, and metrics so the next planning pass is
based on what happened, not on the agent's summary.

## Adapt

Use what you measured to decide the next move:

- close the bead when the evidence passed
- refine the bead when the task was underspecified
- update the spec when the requirement changed
- create follow-up beads for new work
- stop when the outcome is reached

### Optional: Project-Level Routing Override

Projects that need to pin a specific harness, model, or endpoint can author
`.ddx/config.yaml` with an `agent:` block (see [Run Architecture](../concepts/run-architecture/)
and [Skills](../skills/)). This is an advanced override, not a prerequisite:
the zero-config flow above is the default path.

## Run

```bash
ddx run --harness claude --prompt task.md
ddx work --once
```

For side-by-side prompt comparison, use `compare-prompts`. For critique-driven
multi-model review, use `adversarial-review`.

## Update

Check for updates:

```bash
ddx upgrade --check          # Check the DDx binary
ddx upgrade                  # Upgrade the DDx binary
ddx plugin sync              # Recreate generated plugin adapters
ddx plugin install helix --force
```

## Next Steps

- [Operator Loop](../concepts/operator-loop/) — the DDx Plan -> Execute ->
  Measure -> Adapt model
- [CLI reference](../cli) — all commands
- [Ecosystem](../ecosystem) — how DDx fits with HELIX and other tools
- [Creating plugins](../plugins) — add your own workflow to the registry
