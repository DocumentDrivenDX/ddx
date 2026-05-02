# Prompt: author a MET-* metric artifact

You are creating a new metric artifact (`MET-NNN`) under `docs/metrics/`
or another configured artifact root. The artifact must comply with the
schema defined in FEAT-005 §"Type-specific metadata example: MET" and the
runtime contract in TD-005.

## Inputs

- `<short-name>` — kebab-case slug for the metric (e.g. `ddx-test-walltime`)
- `<governing-ids>` — IDs of the artifacts this metric serves (PRD,
  feature spec, technical design)
- `<observable>` — one-sentence description of what is being measured
- `<source>` — `exec` (emitted by a `ddx exec` definition) or `external`

## Steps

1. Allocate a fresh `MET-NNN` ID (use `ddx doc next-id MET` if available;
   otherwise pick the next free integer).
2. Copy `library/artifacts/met/template.md` to the destination (typically
   `docs/metrics/MET-NNN-<short-name>.md`).
3. Fill the frontmatter:
   - `ddx.id` → the allocated ID
   - `ddx.depends_on` → `<governing-ids>`
   - `metric.schema_version` → always `1` for v1
   - `metric.unit` → the actual unit string (no synonyms; pick one and
     stick with it — mixed units are refused by compare and trend)
   - `metric.direction` → `lower-is-better` or `higher-is-better`; must
     match the constants the runtime uses
   - `metric.goal` and `metric.budget` — set whichever you can defend.
     Prefer omitting a field over guessing.
   - `metric.source` → matches `<source>`
   - `metric.scope` → narrowest level at which the value is meaningful
4. Write the prose sections:
   - **What this measures** — define the observable and its boundary
   - **Why it matters** — link the decision back to `depends_on`
   - **How it is observed** — name the producer (`ddx exec` definition
     for `source: exec`; upstream system + cadence for `source: external`)
   - **Goal vs budget** — restate the values and what enforcement they
     get; note that gate `thresholds.ratchet` is authoritative when both
     the gate and `metric.budget` are set
5. If `source: exec`, ensure (or file a follow-up bead to ensure) a
   matching `ddx exec` definition exists with
   `ddx.execution.metric.metric_id: MET-NNN`. The producer must be in
   place before the metric can collect history.
6. Run `ddx doc audit` (or `ddx doc validate`) and fix any new errors
   the new artifact introduces. The metric must be reachable from a
   root via `depends_on`.

## Quality bar

- The frontmatter parses, all required `metric:` fields are present,
  and `schema_version` is exactly `1`.
- `unit` and `direction` are concrete, not placeholders.
- The artifact is reachable from a root in the doc graph.
- If `source: exec`, the metric ID is referenced by at least one
  `ddx exec` definition (or a follow-up bead exists to wire one).
- Prose is specific: a future maintainer can tell what is included
  in the measurement and what is excluded.

## Anti-patterns

- Inventing a third `source` value beyond `exec | external`. `derived`
  is reserved for v2 and must not appear in v1 artifacts.
- Picking a `direction` string that does not match the runtime
  constants (`lower-is-better`, `higher-is-better`).
- Setting `metric.budget` without coordinating with the observing gate's
  `thresholds.ratchet`. They will disagree and `ddx doc validate` will
  warn.
- Mixing units across history rows by editing `unit` after observations
  exist. Retire the metric and create a new one instead.
