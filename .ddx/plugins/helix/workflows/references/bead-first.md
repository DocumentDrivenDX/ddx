# Reference: Bead-First Execution

Every HELIX action that modifies files must be governed by a bead. This
reference defines the bead acquisition pattern that all actions follow.

## Why Bead-First

Without a governing bead, actions modify files ad hoc — there is no plan to
measure against, no acceptance criteria to verify, and no traceable record of
what the action intended to do. Bead-first ensures:

- **Traceability**: Every file change traces back to a bead with a description,
  acceptance criteria, and governing artifact references.
- **Measurability**: After execution, the bead's acceptance criteria define what
  "done" means, and measurement results are recorded on the bead.
- **Feedback**: The report phase creates follow-on beads from measurement
  findings, closing the loop between the execution helix and the planning helix.

## Bead Acquisition Pattern

Every action (except `triage` and `check`) includes a Phase 0.5 — Bead
Acquisition immediately after bootstrap:

### 1. Search for an existing governing bead

```bash
ddx bead list --status open --label kind:planning,action:<name> --json
```

If the action was dispatched with a specific scope (e.g., `helix design auth`),
filter by scope or `spec-id` to find a bead that governs this exact work.

If a matching open bead exists:
- Verify it is still relevant (not stale, not superseded).
- Claim it: `ddx bead update <id> --claim`

### 2. Create a new governing bead if none exists

```bash
ddx bead create "<action>: <scope description>" \
  --type task \
  --labels helix,phase:<appropriate-phase>,kind:planning,action:<name> \
  --set spec-id=<governing-artifact> \
  --description "<context-digest>...</context-digest>
<action-specific description of what this pass will do>" \
  --acceptance "<what done means for this action>"
```

The bead description must include:
- A `<context-digest>` assembled per `.ddx/plugins/helix/workflows/references/context-digest.md`
- The action's inputs and scope
- References to governing artifacts

The acceptance criteria must be specific and verifiable. Examples:

| Action | Example acceptance criteria |
|--------|---------------------------|
| `design` | "Design document converged with all required sections including concern-mandated sections; written to canonical path" |
| `polish` | "All plans in scope decomposed into beads; convergence reached (< 3 changes for 2 consecutive rounds); context digests refreshed" |
| `evolve` | "Requirement threaded through all affected artifacts; no unresolved conflicts; downstream beads created" |
| `align` | "Alignment review complete; all gaps classified; execution issues created for real gaps" |
| `backfill` | "Missing artifacts reconstructed; assumption ledger complete; follow-up issues created for guidance-dependent items" |
| `build` | (already bead-driven — uses the execution bead directly) |
| `review` | "All review passes complete; findings filed as beads; AGENTS.md updated if needed" |
| `frame` | "Artifacts created/updated per type requirements; downstream design issues filed" |

### 3. All subsequent file modifications are governed by this bead

After acquisition, the bead ID is the action's anchor. Commit messages
reference it. Measurement records against it. Report closes it.

## Label Convention

Planning-helix beads use two labels:

- `kind:planning` — distinguishes planning work from execution work
- `action:<name>` — identifies which action will execute this bead

Combined with the standard `helix` label and phase labels, a typical planning
bead has labels: `helix,kind:planning,action:design,phase:build`.

Execution beads (those consumed by `helix build`) do not carry `kind:planning`
or `action:*` labels — they carry `phase:build`, `phase:deploy`, or
`phase:iterate` as before.

## Exceptions

### Triage

`helix triage` creates beads — it is the entry point that bootstraps the bead
graph. Requiring triage to have its own governing bead would be infinite
regress. Triage is exempt from bead acquisition.

### Check

`helix check` is read-only. It does not modify files and does not require a
governing bead. It reads both helices and recommends which one needs attention
next.

### Operator-created beads

When an operator (human or outer agent) dispatches an action, they may create
the governing bead themselves via `ddx bead create` or `helix triage` before
invoking the action. In this case, the action's bead acquisition phase finds
the existing bead rather than creating a new one. This is the preferred pattern
for deliberate work — the operator decides what to do (planning helix), then
the agent executes it (execution helix).

## Lifecycle

```
create/find → claim → execute → measure → report → close
```

1. **Create/find**: Phase 0.5 of every action.
2. **Claim**: `ddx bead update <id> --claim` to prevent concurrent work.
3. **Execute**: The action's main phases (Phase 1 through N).
4. **Measure**: Verify acceptance criteria; record results on the bead.
5. **Report**: Create follow-on beads; close the governing bead with evidence.
6. **Close**: `ddx bead close <id>` with a summary of what was done.

See `.ddx/plugins/helix/workflows/references/measure.md` and `.ddx/plugins/helix/workflows/references/report.md`
for the measure and report phases.
