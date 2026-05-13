# Document-Driven Development Experience (DDx)

DDx — Document-Driven Development Experience — is the tooling and
methodology layer that keeps governing documents current and uses them
to drive AI agents through software development.

The core insight: AI agents work best when they have structured, up-to-date
context. DDx provides that context through a maintained artifact graph and
the tools to keep it honest — so agents always know *what* to build, *why*,
and *how*, without human briefings or stale chat history.

## Core Principles

1. **Documents govern code.** Requirements, designs, and test plans are
   authoritative. Source code is evidence of implementation, not source of
   truth. When documents and code disagree, the document wins until
   explicitly amended.

2. **Authority flows downward.** Artifacts form a strict hierarchy. Higher
   artifacts govern lower ones absolutely. Conflicts resolve by deferring
   to the higher-authority artifact — never by silently overriding it.

3. **Specification before implementation.** Define the *what* and *why*
   before the *how*. Write acceptance criteria before tests. Write tests
   before code. Each layer constrains the next.

4. **Evolution is symmetric.** Adding a capability, removing a component,
   and amending a constraint all require the same top-down propagation
   through every affected artifact. The doc-first rule applies to removals
   and exclusions equally.

5. **Tests are executable specifications.** They encode design intent as
   assertions. Tests drive implementation but do not override the
   requirements or designs they encode — they are a bridge, not a source
   of authority.

6. **The artifact graph is the context.** An agent or developer entering
   a DDx project can build complete understanding from the artifacts alone.
   No tribal knowledge, no chat history, no implicit assumptions. The
   documents *are* the briefing.

## The Authority Hierarchy

DDx defines a strict precedence order for artifacts. When two artifacts
disagree, the one closer to the top governs:

| Level | Artifact Layer | Establishes |
|-------|---------------|-------------|
| 1 | Vision | Why the project exists, who it serves, what success looks like |
| 2 | Requirements | What to build, prioritized and scoped |
| 3 | Feature Specs / User Stories | Acceptance criteria per capability or behavior |
| 4 | Architecture / Decisions | System structure, binding constraints, rationale for choices |
| 5 | Solution & Technical Designs | Approach per feature or story |
| 6 | Test Plans / Executable Tests | Verification strategy and executable specs |
| 7 | Implementation Plans | Build sequencing and dependency order |
| 8 | Source Code / Build Artifacts | The running system |

Rules:

- Higher layers govern lower layers absolutely.
- Tests govern build execution but do not override requirements or design.
- Source code reflects current state but does not redefine the plan.
- If implementation conflicts with a governing artifact, fix the
  implementation — or explicitly amend the artifact first.

## Artifact Lifecycle

Every DDx artifact follows three states:

```
        create          validate         implement
 ────────►  DRAFT  ────────►  GOVERNING  ────────►  REALIZED
                         ▲                    │
                         │    evolve          │
                         └────────────────────┘
```

- **Draft** — Written but not yet validated against higher-authority
  artifacts. May contain inconsistencies.
- **Governing** — Consistent with the authority hierarchy and actively
  directing downstream work.
- **Realized** — Downstream artifacts and code fully satisfy this
  artifact's requirements. Tests pass. Acceptance criteria met.

State is inferred, not declared. A feature spec with no corresponding
design is framed but not designed. A design with passing tests is realized.
An alignment audit that flags drift sends a realized artifact back to
governing for evolution.

## The Evolution Model

Artifacts are living documents. When requirements change, the change must
propagate through the full artifact stack:

1. **Analyze** — Classify the change: addition, removal, amendment,
   new constraint, incident remediation.
2. **Discover** — Walk the artifact graph to identify every document in
   the blast radius.
3. **Detect conflicts** — Compare the proposed change against existing
   artifacts and in-flight work. Flag contradictions.
4. **Propagate top-down** — Amend artifacts in authority order, highest
   first. A vision change flows to requirements, then specs, then designs,
   then test plans. Never update a lower artifact in a way that contradicts
   a higher one already amended.
5. **Decompose** — Derive implementation work items from the artifact
   changes. Each item should be independently verifiable.
6. **Wire dependencies** — Link new work to existing in-flight items.
   Mark supersessions where new work replaces obsolete items.
7. **Report** — Record what changed, what conflicted, and what needs
   human resolution.

This model ensures that a scope reduction (dropping a subsystem) receives
the same rigor as a scope expansion (adding a feature). Nothing is removed
by simply deleting code.

## Phases

DDx organizes work into phases that correspond to the authority hierarchy.
Each phase produces artifacts at its level before the next phase begins:

| Phase | Purpose | Artifacts Produced |
|-------|---------|-------------------|
| **Discover** | Validate the opportunity | Vision, business case, competitive analysis |
| **Frame** | Define the problem | Requirements, feature specs, user stories, principles |
| **Design** | Architect the solution | Architecture, ADRs, solution designs, technical designs, contracts |
| **Test** | Specify verification | Test plans, test procedures, executable tests |
| **Build** | Implement the solution | Source code, work items, build artifacts |
| **Deploy** | Release and operate | Deployment checklists, monitoring setup, runbooks, release notes |
| **Iterate** | Learn and improve | Alignment reviews, retrospectives, metrics |

Phases are not strictly sequential — iteration sends realized artifacts
back through evolution, and early phases may be revisited when assumptions
prove wrong. But the authority order holds regardless of which phase you
are in: vision still governs code even during iteration.

## Agent Context Model

DDx solves the cold-start problem for AI agents. An agent entering a DDx
project does not need a briefing, a knowledge transfer, or chat history.
It reads the artifact graph.

**What the agent reads:**
- Project entry point (e.g., AGENTS.md) for structure and conventions
- Upstream artifacts (vision → requirements → specs) for *what* and *why*
- Design artifacts for *how*
- Test plans for verification strategy
- Work queue for *what to do next*

**How context stays current:**
- **Evolution** propagates requirement changes to all affected artifacts
- **Alignment audits** catch drift between artifacts and reality
- **Traceability** links every work item to its governing spec
- **Quality ratchets** prevent metric regression across iterations
- **Bounded actions** give agents clear entry/exit criteria per operation

**Why this works:**
- No implicit knowledge — everything is in versioned files
- Deterministic conflict resolution via the authority hierarchy
- Navigable traceability in both directions (spec → code, code → spec)
- Agents can enter at any point in the lifecycle and orient themselves

## DDx Tooling

DDx provides concrete tools for managing work items and design artifacts.
These are workflow-agnostic — any methodology can layer its own semantics
on top.

### Beads

Beads are portable, ephemeral work items with metadata. The name follows
the `bd` (Dolt-backed) and `br` (SQLite-backed) convention: short tool
names for the same conceptual unit. DDx beads use JSONL-backed local
storage; `bd` and `br` provide database-backed alternatives. All three
interchange via JSONL.

```
ddx bead create "Fix auth bug" --type bug --priority 1
ddx bead list --status open
ddx bead ready                    # Open beads with all deps satisfied
ddx bead dep add <id> <dep-id>    # Declare ordering constraints
ddx bead import --from bd         # Import from external bead stores
ddx bead export --stdout          # JSONL interchange
```

Beads carry between projects and tools. Unknown fields are preserved on
round-trip — this is how workflows like HELIX layer `spec-id`,
`execution-eligible`, and other workflow-specific metadata without DDx
needing to understand them.

Workflows register validation hooks (`.ddx/hooks/validate-bead-create`)
to enforce their own rules on top of DDx's base validation.

### Architecture Decision Records

```
ddx adr create "Use PostgreSQL for persistence"
ddx adr list [--status proposed]
ddx adr show ADR-001
ddx adr validate --all
```

ADRs capture binding decisions with context, rationale, alternatives, and
consequences. DDx scaffolds them with correct ID allocation, `ddx.id` /
`ddx.depends_on` frontmatter, and required section structure.

### Solution Designs

```
ddx sd create "User auth flow" --feature FEAT-002
ddx sd list
ddx sd show SD-001
ddx sd validate --all
```

Solution designs describe the chosen approach for a feature — scope,
acceptance criteria, solution approaches, and component changes.

### Workflow Capabilities

Beyond these core tools, a DDx-based workflow needs higher-level
capabilities that orchestrate them:

| Capability | Purpose |
|-----------|---------|
| **Frame** | Create upstream artifacts (vision, requirements, specs) |
| **Design** | Refine solution and technical designs |
| **Triage** | Decompose artifacts into beads |
| **Build** | Execute one bead against its governing artifacts |
| **Evolve** | Thread a requirement change through the artifact stack |
| **Align** | Audit artifact consistency and flag drift |
| **Review** | Post-implementation verification |
| **Check** | Route to the appropriate next action |

These capabilities are workflow-specific. HELIX provides all of them;
other workflows may implement a subset.

## HELIX

HELIX is the reference implementation of DDx. It layers workflow-specific
semantics on top of DDx tools:

- **Phase labels** (`phase:build`, `phase:design`, etc.) on beads
- **Spec-id enforcement** linking every bead to its governing artifact
- **Execution-eligible** derivation from phase labels
- **Supervisory run loop** that sequences build/check/design/review
- **Agent orchestration** dispatching to Claude, Codex, virtual (recorded replay), or other agents
- **Quality ratchets** preventing metric regression

DDx provides the beads, ADRs, and SDs. HELIX provides the workflow that
drives agents through them.

See [Workflow Contract](README.md) for the HELIX-specific operating model.
