# HELIX Action: Evolve

You are threading a new requirement through the project's artifact stack.

Your goal is to take a requirement — a feature request, constraint change,
spec amendment, incident learning, or customer feedback — and propagate it
through the governing documents, detect conflicts with existing work, create
properly triaged tracker issues for implementation, and produce a structured
evolution report.

This action modifies governing artifacts. Treat every write with the same
care as a production code change.

## Action Input

You receive:

- A requirement description (natural language or structured)
- Optional: `--scope` to limit blast radius (e.g., `area:wal`, `SD-017`)
- Optional: `--artifact` to target a specific governing artifact
- Optional: `--from` to indicate the requirement source (incident, feedback, etc.)

## Authority Order

When artifacts disagree, use this precedence:

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution Designs / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

Update higher-authority documents FIRST, then propagate downward. Never
update a lower-authority artifact in a way that contradicts a higher one.

## PHASE 0 — Bootstrap

0. **Load active design principles** following `.ddx/plugins/helix/workflows/references/principles-resolution.md`.
   Use these as scoping guidance when evaluating which artifacts need updates
   and how to resolve judgment calls. Note: `helix evolve` reads principles
   but never modifies the principles file — only `helix frame` may write it.
0a. **Load active concerns and practices** following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
   Concern context affects the scope of downstream changes:
   - A requirement that implies a technology change (new language, new framework,
     new runtime, new dependency) must be checked against declared concerns.
   - If the change conflicts with an active concern (e.g., adding a Node.js
     dependency in a `typescript-bun` project, or introducing `println!` logging
     in an `o11y-otel` project), flag it as a conflict requiring an ADR.
   - If the requirement implies adopting a technology that has a library concern
     but the project doesn't yet declare it, include concern selection as part of
     the evolution — create or update `docs/helix/01-frame/concerns.md` alongside
     the other artifact updates.
   - If the requirement removes or replaces a technology covered by an active
     concern, the concern declaration and project overrides must be updated as
     part of Phase 4 artifact evolution.
0b. **Context digest**: When `helix evolve` creates or modifies beads, it must
   assemble a context digest per `.ddx/plugins/helix/workflows/references/context-digest.md` and
   prepend it to the bead description. If a repo helper exists for digest
   assembly, use it instead of hand-writing the XML. The `<concerns>` element
   must contain matched concern names, never `area:*` labels.

## PHASE 0.5 — Bead Acquisition

Before modifying any artifacts, acquire a governing bead for this evolution
pass. See `.ddx/plugins/helix/workflows/references/bead-first.md` for the full pattern.

1. Search for an existing open bead governing this work:
   - `ddx bead list --status open --label kind:planning,action:evolve --json`
2. If found, verify it is still relevant and claim it:
   - `ddx bead update <id> --claim`
3. If not found, create one:
   ```bash
   ddx bead create "evolve: <requirement summary>" \
     --type task \
     --labels helix,phase:design,kind:planning,action:evolve \
     --description "<context-digest>...</context-digest>
   Thread requirement through artifact stack: <requirement description>.
   Source: <--from value if provided>" \
     --acceptance "Requirement threaded through all affected artifacts; no unresolved conflicts; downstream beads created with context digests"
   ```
4. Record the bead ID. All subsequent artifact modifications are governed by
   this bead.

## PHASE 1 — Requirement Analysis

Parse the input requirement:

1. Identify the core change — what capability, constraint, or behavior is
   being added, changed, or removed.
2. Classify the type: new feature, amendment, constraint, policy change,
   incident remediation.
3. Assess scope: which subsystems, interfaces, and data models are affected.
4. If the requirement is ambiguous, state your interpretation explicitly
   and proceed. Do not halt for clarification on things you can reasonably
   infer.

## PHASE 2 — Artifact Discovery

Search the project's doc tree for governing artifacts in scope:

1. Read `AGENTS.md` for the project's artifact structure.
2. Search for artifacts that reference the affected subsystems, interfaces,
   or capabilities.
3. Build an ordered list of artifacts to review, highest authority first.
4. For each artifact, note its current state and the section(s) that will
   need updating.
5. **If the evolution will create new numbered artifacts**, load the relevant
   meta.yml files and determine the next available IDs now — before writing
   anything:
   - **New feature spec (FEAT-NNN)**: read
     `.ddx/plugins/helix/workflows/phases/01-frame/artifacts/feature-specification/meta.yml`,
     scan `docs/helix/01-frame/features/FEAT-*.md`, set next FEAT ID = max + 1
     (use `001` if none exist).
   - **New solution design (SD-NNN)**: read
     `.ddx/plugins/helix/workflows/phases/02-design/artifacts/solution-design/meta.yml`,
     scan `docs/helix/02-design/solution-designs/SD-*.md`, set next SD ID =
     max + 1.
   - **New technical design (TD-NNN)**: read
     `.ddx/plugins/helix/workflows/phases/02-design/artifacts/technical-design/meta.yml`,
     scan `docs/helix/02-design/technical-designs/TD-*.md`, set next TD ID =
     max + 1.
   - Record these values. Use them exclusively when assigning IDs; never guess
     or reuse an existing number.

Use commands like:
- `find docs/ -name "*.md" | xargs grep -l "keyword"`
- `ddx bead list --json` to find related open issues

## PHASE 3 — Conflict Detection

For each affected artifact:

1. Read the relevant sections.
2. Check whether the new requirement contradicts existing content.
3. Check whether the new requirement conflicts with open tracker issues.
4. Check whether the new requirement conflicts with active concerns:
   - Does it introduce a tool, framework, or convention that contradicts a
     declared concern's constraints or practices?
   - Does it require a technology not covered by any active concern?
   - Would it change the project's area taxonomy or concern applicability?
5. For each conflict found, record:
   - Artifact and section (or concern name, for concern conflicts)
   - What the artifact or concern currently says
   - What the requirement asks for
   - Whether the conflict is resolvable by reasonable interpretation or
     requires human decision
   - For concern conflicts: whether an ADR is needed to justify the departure

Conflicts that require human decision are flagged but do NOT halt the
entire evolution. Proceed with non-conflicting artifacts.

When a concern conflict is identified, the resolution options are:

- **ADR + concern update**: Create an ADR justifying the technology change,
  then update `concerns.md` to reflect the new selection.
- **Project override**: Add or modify a project override in `concerns.md`
  that documents the exception and its rationale.
- **Reject**: The requirement is incompatible with the project's technology
  commitments and should be reconsidered.

## PHASE 4 — Artifact Evolution

For each non-conflicting artifact, in authority order (highest first):

1. Read the current document.
2. Draft the amendment — add, modify, or extend the relevant sections.
3. Validate the amendment does not contradict any higher-authority artifact
   that was already updated in this session.
4. For any **new** artifact being written (not an update to an existing file):
   - Confirm the ID was assigned from the scanned-next-ID computed in Phase 2,
     not guessed.
   - Inspect the ddx frontmatter `depends_on` list. For each referenced ID,
     verify the target artifact exists on disk before writing. If a target is
     missing, either remove the dependency or stop and request guidance. Never
     write an artifact with a broken `depends_on` reference.
5. Write the update.
6. If the project uses acceptance manifests (`.acceptance.toml`), update
   those too with new or modified acceptance criteria.
7. If the update touches acceptance artifacts, set
   `NIFLHEIM_ACCEPTANCE_CHANGE=1` before committing.

Keep amendments minimal and scoped. Do not rewrite sections that aren't
affected by the requirement.

## PHASE 5 — Issue Decomposition

Create tracker issues for the implementation work implied by the updated
artifacts:

1. For each updated artifact, determine what code changes are needed.
2. Create issues that are individually implementable in one `helix build`
   cycle.
3. Each issue must pass triage validation:
   - `--labels helix,phase:build,...`
   - set `spec-id` with `--set spec-id=<updated-artifact>`
   - `--acceptance` with deterministic criteria
4. Set `--parent` if the issues belong to an existing epic.
5. Group related issues under a new epic if the requirement implies
   multiple implementation slices.

## PHASE 6 — Dependency Wiring

Search existing open issues for overlap with the new issues:

1. `ddx bead list --status open --json` to get the current queue.
2. For each new issue, check if existing issues touch the same files,
   subsystems, or acceptance criteria.
3. Add dependencies with `ddx bead dep add` where ordering matters.
4. If the new requirement supersedes an existing issue, mark it with
   `ddx bead update <id> --superseded-by <new-id>`.

## PHASE 7 — Evolution Report and Commit

1. Commit all artifact changes with a message referencing the requirement
   and the governing bead ID.
2. Push to the remote.

## PHASE 8 — Measure

Verify the evolution against the governing bead's acceptance criteria.
See `.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

1. **Artifact consistency**: All updated artifacts are consistent with each
   other and with higher-authority artifacts.
2. **Conflict resolution**: All resolvable conflicts were resolved; remaining
   conflicts are documented with clear resolution options.
3. **Issue completeness**: Downstream implementation beads were created for all
   code changes implied by the updated artifacts.
4. **Concern threading**: If the requirement introduced or changed a concern,
   verify the concern is properly declared and its practices are referenced
   in new beads.
5. **Record results** on the governing bead:
   `ddx bead update <id> --notes "<measure-results>...</measure-results>"`

## PHASE 9 — Report

Close the evolution cycle and feed back into the planning helix.
See `.ddx/plugins/helix/workflows/references/report.md` for the full pattern.

1. If measurement passed, close the governing bead with evidence summary.
2. If conflicts remain, create follow-on beads for each unresolved conflict
   requiring human decision.
3. Output a structured report:

```
## Evolution Report

### Requirement
[summary of the requirement]

### Artifacts Updated
- [artifact]: [what changed]

### Artifacts Skipped (Conflicts)
- [artifact]: [conflict description — what needs human resolution]

### Issues Created
- [id]: [title] (spec-id: [ref], deps: [ids])

### Dependencies Wired
- [new-id] depends on [existing-id]: [reason]

### Open Conflicts Requiring Human Resolution
- [description with specific artifact references]
```

4. Output the required trailer:

```
EVOLUTION_STATUS: COMPLETE|CONFLICTS|BLOCKED
ARTIFACTS_UPDATED: [count]
ISSUES_CREATED: [count]
CONFLICTS: [count]
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <governing-bead-id>
FOLLOW_ON_CREATED: N
```

`COMPLETE` means all artifacts updated and issues created with no conflicts.
`CONFLICTS` means some artifacts were skipped due to conflicts that need
human resolution, but non-conflicting work was completed.
`BLOCKED` means the requirement fundamentally contradicts the project's
governing artifacts and cannot proceed without human decision.
