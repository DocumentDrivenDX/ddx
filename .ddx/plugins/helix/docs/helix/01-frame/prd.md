---
ddx:
  id: helix.prd
---
# Product Requirements Document

## Summary

HELIX is a software development methodology and artifact catalog for
AI-assisted teams. It ships portable content (templates for ~32 artifact
types, authoring prompts, methodology documentation) plus a single skill
that keeps the governing artifacts aligned. The primary user experience is
invoking the alignment skill against a project's existing documents and
getting a plan for the changes needed to keep them coherent.

HELIX does not provide a CLI, a tracker, an execution loop, or any runtime
infrastructure. Those are runtime concerns. DDx is the reference runtime;
Databricks Genie and Claude Code are target runtimes. HELIX is the
discipline; the runtime is the engine.

## Problem and Goals

### Problem

AI-assisted software development is now the default practice, but the practice
itself is unpracticed. Teams using AI agents to write production code see three
failure modes:

1. **Artifact drift.** Specifications, designs, and tests fall out of sync with
   code because no one walks the artifacts between work sessions.
2. **Inconsistent agent behavior.** Without a shared methodology, agents make
   locally-good decisions that conflict with global intent — a feature gets
   built, but its design contradicts an earlier ADR; a refactor lands without
   updating the technical design that documented its predecessor.
3. **Lossy session boundaries.** Each new conversation re-improvises the same
   workflow. Specs that should be written are not; reviews that should happen
   do not. Context evaporates.

Human teams developed disciplines for these problems decades ago: PRDs, design
documents, test plans, alignment reviews. The agents that now write our code
have not been given that discipline in a portable, tool-agnostic form.

### Goals

1. **Catalog.** Define a minimal, complete catalog of artifact types covering
   software development from intent through deployment to feedback. Each type
   carries a template, an authoring prompt, and quality criteria.
2. **Alignment skill.** Provide a single skill that any AI agent can run
   against a project's governing artifacts to identify drift, gaps, and
   contradictions, and produce a plan for closing them.
3. **Methodology spec.** Specify authority order, the seven-activity control
   loop, the artifact-type schema, and the alignment skill contract as a
   runtime-neutral document.
4. **Stay small.** HELIX is content + one skill. Not a tool, not a platform,
   not a CLI. Resist scope creep into runtime territory.

### Success Metrics

| Metric | Target | Measurement Method | Timeline |
|---|---|---|---|
| Runtime breadth | 3 documented runtime deployments (DDx, Databricks Genie, one other) | Release audit | 12 months |
| Adoption | 50+ repos using HELIX templates as their artifact catalog | GitHub search; community survey | 18 months |
| Alignment quality | Healthy artifact sets average < 3 findings per skill run | Periodic sample across reference repos | Ongoing |
| Authoring quality | HELIX-template PRDs pass first-pass review at higher rates than free-form equivalents | Comparative review study | 12 months |
| Skill portability | Zero runtime-specific commands in skill body | Automated check on every release | Ongoing |

### Non-Goals

- HELIX will not provide a CLI, tracker, queue, or execution engine.
- HELIX will not impose technology choices or fork by language ecosystem.
- HELIX will not replace product, design, or architectural judgment.
- HELIX will not be a fully autonomous coding agent.
- HELIX will not flatten the seven-activity loop into one generic prompt.

## Users and Scope

### Primary Persona: HELIX Adopter

**Role**: Tech lead, staff engineer, or principal engineer guiding a team's
adoption of AI-assisted development.
**Goals**: Establish a methodology that produces durably reviewable software
output from agents. Reduce the cost of onboarding new team members or new
agent runtimes.
**Pain Points**: Inconsistent agent output across sessions; artifact drift;
the cost of writing a methodology from scratch for each project.

### Secondary Persona: AI Agent

**Role**: Any AI runtime (Claude Code, Genie, Cursor, etc.) executing work
against a HELIX-governed artifact set.
**Goals**: Have a defined set of artifacts to read, a defined set to produce,
and a single skill to invoke for alignment.
**Pain Points**: Ambiguous context, drift between sessions, no standard for
"is this work complete."

### Tertiary Persona: HELIX Maintainer

**Role**: Maintainer of the HELIX content catalog, the methodology spec, and
the alignment skill.
**Goals**: Keep the catalog tight; keep the methodology coherent; ship
portable improvements.
**Pain Points**: Resisting feature creep into runtime territory; pruning
legacy surfaces that crept across the boundary.

### In Scope

- Artifact-type catalog (templates, prompts, quality criteria, examples)
- Methodology specification (authority order, seven-activity loop, principles)
- Single alignment skill (find drift; produce a plan)
- Runtime-portable packaging of all of the above (per-runtime packages for
  DDx, Databricks Genie, Claude Code)
- HELIX's own self-application — its development artifacts in `docs/helix/`
  are themselves authored from HELIX templates

### Out of Scope

- CLI, tracker, queue, execution loop, IDE integration
- Runtime-specific tooling beyond minimal per-runtime packages
- Domain-specific extensions (HELIX is general-purpose)

## Requirements

### Functional Requirements

#### P0

**R-1: Artifact-type catalog.** HELIX ships templates, prompts, and metadata
for every activity in the seven-activity loop. Each artifact type carries a
template (markdown skeleton), a prompt (authoring guidance), quality criteria,
and an example.

**R-2: Authority-ordered relationships.** Every artifact type declares its
position in the authority order. Higher-level artifacts govern lower-level
ones; conflicts resolve up.

**R-3: Alignment skill.** A single skill, deployable to any runtime that can
read and write markdown, takes an artifact root path plus an optional intent
and produces:
(a) a structured alignment report listing gaps, drift, and contradictions in
    the governing artifacts;
(b) a plan describing the artifact updates needed to close them, ordered by
    authority.

**R-4: Runtime-neutral content.** The catalog and skill body contain no
references to specific runtime commands, file layouts, or runtimes beyond
the minimal "read markdown, write markdown, search files" contract.

**R-5: Methodology specification.** A runtime-neutral specification document
defines the authority order, the seven-activity loop, the artifact-type
schema, and the alignment skill contract. New runtime consumers can
implement HELIX compliantly from this spec alone.

**R-6: Self-application.** HELIX's own development artifacts in `docs/helix/`
are authored from HELIX templates. The repo dogfoods its own catalog;
inconsistencies in the dogfood are themselves alignment findings.

#### P1

**R-7: Distribution packages for each target runtime.** HELIX ships as a DDx
plugin, a Databricks Genie skill, and a Claude Code skill. The source content
is the same in every case; the package metadata around it differs per
runtime. Packages are not forks.

**R-8: Quality criteria as deterministic checks.** Where artifact quality
criteria are deterministic (regex patterns, section presence, word counts),
they are expressed in a form that runtimes with prose-checker support
(notably DDx via `ddx doc prose`) can enforce.

**R-9: Per-artifact-type rule scoping.** Quality criteria attach to artifact
types; alignment skill applies type-scoped checks to instances based on ID
prefix or path. Requires upstream DDx feature (file separately on DDx queue).

#### P2

**R-10: Catalog versioning.** Artifact-type changes are reviewable; consumers
can pin to a known catalog version.

**R-11: Onboarding deliverable.** A guided first-project walkthrough usable
without the creator present. The new-teammate review is a release criterion.

### Acceptance Test Sketches

**R-1 (catalog):** Given a fresh repo with no HELIX content, when HELIX
content is installed, then 7 activity directories exist under `workflows/
phases/`, each with at least one artifact-type directory; each artifact-type
directory contains `template.md`, `prompt.md`, `meta.yml`, and an example.

**R-3 (alignment skill):** Given a project with vision, PRD, feature specs,
but no design artifacts, when the alignment skill runs, then the output
report identifies the design gap and the plan describes which solution
designs to author, in what order, with traceability back to the affected
features.

**R-4 (runtime-neutral):** Given the alignment skill source, when grep is
run for runtime-specific commands (`ddx `, `helix `, `bead`, `.helix/`),
then zero hits in the skill's normative body. References are allowed only in
per-runtime package metadata (the DDx-plugin manifest, the Genie skill
descriptor, etc.).

**R-6 (self-application):** Given the HELIX repo, when the alignment skill is
run against `docs/helix/`, then the resulting report has fewer findings than
on the same date one quarter prior — i.e. the dogfood improves over time.

### Technical Context

HELIX content is authored in Markdown with YAML frontmatter using the `ddx:`
namespace (an open schema HELIX adopts; DDx happens to consume it). The
alignment skill is authored in Markdown with frontmatter compatible with
mainstream agent skill formats (Claude Code-compatible). Artifact templates
and prompts are plain Markdown.

HELIX ships per-runtime packages around the same source content:

- **DDx plugin** — installed with `ddx install helix`; the catalog and skill
  show up under DDx's plugin mechanism.
- **Databricks Genie skill** — the content + skill bundled in Genie's skill
  format.
- **Claude Code skill** — the content + skill bundled as a Claude Code skill.

HELIX does not impose a programming language, framework, or toolchain on the
projects that adopt it. The repo itself uses Markdown + YAML; each per-runtime
package uses whatever metadata format its target requires.

### Constraints, Assumptions, Dependencies

**Constraints:**
- HELIX cannot assume any particular runtime is present at
  runtime. The skill must work given only "read file, write file, search
  files" as runtime primitives.
- The skill body cannot reference runtime-specific commands.
- The catalog cannot fork by language ecosystem.

**Assumptions:**
- AI agents capable of executing the alignment skill have file-read/write
  tools and reasonable context window for an governing artifacts (typically tens of
  files).
- Packages can package markdown content for their runtime; the
  packaging mechanics vary but the source content is uniform.

**Dependencies:**
- **DDx (reference runtime).** HELIX uses DDx for its own bead tracking
  and execution. This is HELIX-using-DDx, not HELIX-coupled-to-DDx — the
  relationship is the same one Databricks users would have with their own
  runtime.
- **Markdown/YAML toolchain.** No specialized parser beyond standard YAML
  frontmatter.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Methodology adoption is slow because "just write the code" is the durable default | High | High | Keep HELIX minimal; the alignment skill must produce value on the first run; ship a strong getting-started narrative |
| Runtime fragmentation makes package maintenance unsustainable | Medium | High | Define minimal runtime contract early; small packages, not forks; treat runtime breadth as a release-quality metric |
| Alignment skill produces noisy or wrong findings | Medium | High | Skill is HELIX's primary development focus; quality criteria in templates are explicit and testable; ratchets on finding precision |
| Old HELIX work leaks runtime coupling into new content | Medium | Medium | Decoupling beads in the queue; automated check on each release for runtime refs in skill body |
| Documentation staleness | Medium | Medium | The alignment skill itself catches stale documents; HELIX dogfoods its own catalog |
| Transferability — HELIX cannot be taught without its creator present | Medium | High | Onboarding doc is P1 deliverable; new-teammate review gates the next release |

## Open Questions

- What is the canonical name and location of the alignment skill? (Working
  name: `skills/helix-align/`. Could also be `helix-plan`. Single source of
  truth either way.)
- What is the minimal runtime contract for an alignment-capable runtime?
  (Probably: file read, file write, file search, optional shell exec. Needs
  written spec.)
- How does HELIX express type-scoped quality rules before DDx's prose-checker
  supports artifact-type scoping? Interim approach: prose guidance in
  `prompt.md`. Long-term: structured `rules:` block in `meta.yml` once DDx
  supports it.
- Which legacy FEATs are superseded by this PRD? Candidates:
  `FEAT-001-helix-supervisory-control`, `FEAT-002-helix-cli`,
  `FEAT-005-execution-backed-output`, parts of `FEAT-004-plugin-packaging`,
  parts of `FEAT-011-slider-autonomy`. Surviving FEATs: `FEAT-003-principles`,
  `FEAT-006-concerns-practices-context-digest`, `FEAT-008-artifact-template-quality`,
  `FEAT-009-team-onboarding`, `FEAT-010-testing-strategy-templates`.
- Does HELIX itself need any tooling beyond the alignment skill (e.g., a
  catalog validator, a release-time portability check)? If yes, where does
  that tooling live — in HELIX, or as a DDx contribution?

## Success Criteria

HELIX is successful when:

1. A new team can adopt the methodology by reading the vision, the PRD, and
   the methodology spec, and installing the content catalog via a runtime
   package — without further guidance from a HELIX maintainer.
2. The alignment skill runs against a real project's governing artifacts and
   produces a plan that a human can review and approve in under 10 minutes.
3. At least three runtimes have working packages; teams can pick the one
   that fits their environment.
4. HELIX's own `docs/helix/` is visibly authored from its own catalog — the
   dogfood is healthy by the alignment skill's own measure.
