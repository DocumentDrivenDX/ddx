---
name: helix-evolve
description: 'Thread any change — addition, removal, or amendment — through the artifact stack. Use when governing docs need updating: requirements change, features are added or dropped, specs are amended, or incidents reveal needed changes.'
argument-hint: '"requirement description" [--scope S] [--artifact ID] [--from source]'
---

# Evolve: Propagate Changes Through the Artifact Stack

When a governing artifact needs to change — whether adding a capability,
removing a component, amending a spec, or responding to an incident — this
skill threads the change through the full artifact stack from vision down
to tracker issues.

**Rule of thumb**: if the change touches or should touch an SD, ADR, TP, or
PRD document, use evolve. This applies equally to additions, removals, and
amendments. The doc-first rule is symmetric — removing a system from a design
document requires the same artifact propagation as adding one.

## When to Use

- A new feature is requested or a capability needs to be added
- A system, dependency, or capability is being **dropped or excluded**
- An incident or bug reveals a missing constraint or policy
- An existing spec needs amendment (new acceptance criteria, changed behavior)
- Customer feedback identifies a gap in the current design
- A dependency or constraint changes (new library, API version, compliance rule)
- **Any change to the set of systems/components in a multi-system design**
  (e.g., dropping a database from a benchmark suite, removing a connector)
- A decision changes scope (V1 → post-V1 deferral, or vice versa)

## What It Does

1. Analyzes the requirement and identifies affected subsystems
2. Discovers which governing artifacts need updating
3. Detects conflicts with existing artifacts and open work
4. Updates artifacts in authority order (vision → specs → designs → test plans)
5. Creates properly triaged tracker issues for implementation
6. Wires dependencies to existing open issues
7. Produces an evolution report with conflicts flagged for human resolution

## Examples

```bash
# New feature requirement
helix evolve "Add WAL compression support with configurable algorithms"

# Scoped amendment
helix evolve --scope area:wal "WAL segments should support LZ4 and zstd compression"

# Target a specific artifact
helix evolve --artifact SD-017 "Verification engine needs loom support for concurrency testing"

# Incident-driven requirement
helix evolve --from incident "INC-042: ingest pipeline needs circuit breakers to prevent cascade failure"

# Removal / exclusion
helix evolve --artifact SD-022 "Drop StarRocks from the competitive benchmark suite — unreliable results"

# Scope change
helix evolve "Defer SD-026 bridge adapters to post-V1"
```

## References

- Action prompt: `workflows/actions/evolve.md`
- Authority order: `workflows/actions/implementation.md` (Authority Order section)
- Triage validation: `scripts/tracker.sh` (tracker_validate_create)
- Tracker conventions: see `ddx bead --help` (DDx FEAT-004)
