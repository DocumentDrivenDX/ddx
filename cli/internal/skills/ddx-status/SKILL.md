---
skill:
  name: ddx-status
  description: Get a combined health and work-item overview for a DDx project.
  args: []
---

# DDx Status: Project Health Overview

This skill collects version info, health checks, and work item state into a
single project snapshot. Run it at the start of a session or whenever you need
orientation on current project state.

## When to Use

- Starting a new work session and orienting to project state
- Checking whether DDx and its dependencies are healthy before running agents
- Getting a summary of open work items and what is actionable now
- Diagnosing a suspected configuration or sync problem

## Steps

### 1. Version and Sync Info

```bash
ddx status
```

Shows DDx version, library sync status, and whether resources are up to date.
Note any warnings about out-of-date resources or sync failures.

### 2. Health Check

```bash
ddx doctor
```

Runs diagnostics on DDx installation and configuration. Identifies missing
dependencies, misconfigured harnesses, credential problems, and other
actionable issues. Fix any errors before proceeding with agent dispatch or
bead operations.

### 3. Work Item Summary

```bash
ddx bead list
```

Shows all open beads. Use this to understand the full scope of in-flight work.

### 4. Actionable Items

```bash
ddx bead ready
```

Shows beads with no unmet dependencies — the items that can be worked on right
now. This is the primary input for selecting the next task to implement.

## Summarizing Findings

After running the four commands, summarize:

1. **Health**: Any errors from `ddx doctor` that must be resolved first
2. **Sync**: Whether library resources are current per `ddx status`
3. **Ready count**: How many beads are actionable from `ddx bead ready`
4. **Blocked count**: How many beads are blocked (visible in `ddx bead list`)

If `ddx doctor` reports errors, address those before starting implementation
work. If library resources are out of date, run `ddx update` to sync.

## References

- Full flag list: `ddx status --help`, `ddx doctor --help`
- Bead queries: `ddx bead --help`
- CLI feature spec: `docs/helix/01-frame/features/FEAT-001-cli.md`
