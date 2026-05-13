---
ddx:
  id: helix.workflow.ratchets
  depends_on:
    - helix.workflow
    - helix.workflow.principles
---
# Quality Ratchets

A ratchet is a quality metric that is only allowed to improve. Once a project
reaches a measured level of quality, the floor is committed to version control
and future changes that regress below that floor are blocked until the
regression is fixed or the floor is explicitly lowered with documented
justification.

Ratchets operationalize three existing HELIX commitments:

- **Continuous Validation**: "Tests checked for coverage and quality" — ratchets
  make this check quantitative and persistent. (Enforced by the Build phase
  enforcer exit gate.)
- **Build Phase Exit Gate**: "Coverage targets met" and "Performance targets
  met" — ratchets define *how* those targets are tracked and prevented from
  regressing.
- **Feedback Integration**: "Metrics inform requirement updates" — ratchet
  trends are iterate-phase inputs that feed back into the next cycle. (Enforced
  by the Iterate phase enforcer.)

## Scope Boundary

This document defines the ratchet *pattern* — what it is, how it works, and
where it connects to HELIX phases and actions. It does not ship enforcement
scripts or fixture files. Those belong in the adopting project because the
measurement tools, languages, and thresholds are project-specific.

HELIX defines the contract shape. Adopting projects wire in their own
measurement commands.

## The Ratchet Pattern

Every ratchet has five components:

1. **Metric** — a quantitative measurement produced by a repeatable command
   (e.g., line coverage percentage, acceptance satisfaction ratio, I/O
   efficiency)
2. **Floor** — the committed minimum acceptable value, stored in a versioned
   fixture file in the project repository
3. **Tolerance** — a band below the floor that absorbs measurement noise
   without triggering a failure (e.g., 1% for coverage, 5% for performance)
4. **Auto-bump** — when the measured value exceeds the floor by a defined
   margin, the floor is automatically raised and committed
5. **Override protocol** — an explicit, auditable process for intentionally
   lowering the floor when a regression is justified

### Invariants

- The floor file is committed to version control. It is not ephemeral.
- The floor only increases unless the override protocol is followed.
- The override protocol requires an issue that documents the justification,
  the new floor, and the acceptance criteria for restoring the previous
  level.
- Measurement commands must be deterministic and reproducible. Flaky metrics
  make ratchets harmful.

### Enforcement Interface

An adopting project provides an enforcement script (or equivalent) that:

- **Inputs**: path to the floor fixture file
- **Outputs**: exit code 0 (pass) or non-zero (fail), plus a machine-readable
  summary (measured value, floor, pass/fail, delta)
- **Side effects**: when the measured value exceeds the auto-bump threshold,
  the script updates the floor fixture file in the worktree (the caller
  decides whether to stage and commit)

HELIX action prompts reference this interface. The adopting project is
responsible for the implementation.

### Floor Fixture File

The format of the floor fixture file is project-specific. Use whatever format
your toolchain naturally emits and consumes — JSON for tool-generated
measurement data, TOML for policy artifacts that humans review and maintain,
YAML if that matches your project conventions.

At minimum, the fixture must contain:

- a version field for schema evolution
- the current floor value(s)
- the tolerance
- a timestamp or commit reference for the last update

The fixture may optionally include a `metric` field referencing a shared metric
definition at `docs/helix/06-iterate/metrics/<name>.yaml`. When present, the
metric definition provides the measurement command, output pattern, and
tolerance — the floor fixture only needs to record the floor value and update
history. See `.ddx/plugins/helix/workflows/templates/metric-definition.yaml` for the schema.

## Three Ratchet Types

HELIX recognizes three ratchet types. A project may adopt any combination.

### Acceptance Criteria Ratchet

**What it measures**: the ratio of acceptance criteria classified as SATISFIED
to total active criteria, as determined by the reconcile-alignment action's
Phase 3 (Acceptance Criteria Validation).

**How it connects to HELIX**: reconcile-alignment already classifies each
criterion as SATISFIED, TESTED_NOT_PASSING, UNTESTED, or UNIMPLEMENTED. The
ratchet makes that classification persistent — a committed floor prevents the
satisfaction count from silently decreasing between alignment reviews.

**Granularity**: per-criterion tracking is preferred over aggregate percentages.
The floor fixture should record individual criterion states so that a
regression can be traced to a specific criterion, not just observed as a
percentage drop.

**State transitions**: criteria move through a `planned` to `active` lifecycle.
A `planned` criterion is design intent that does not yet require test coverage.
An `active` criterion must have at least one test that exercises it. The
transition from `planned` to `active` happens when implementation ships with
passing tests. Active criteria cannot revert to `planned` without the override
protocol.

**Co-staging**: when the floor fixture is updated, the sibling planning
artifact (test plan, acceptance manifest, or equivalent) should be staged in
the same commit to maintain traceability.

### Test Coverage Ratchet

**What it measures**: source-line coverage percentage (or equivalent) produced
by the project's test suite.

**How it connects to HELIX**: the Build phase enforcer requires "coverage
targets met" as an exit gate. The ratchet defines what the target is, prevents
it from regressing, and auto-bumps it when coverage improves.

**Measurement**: use the project's native coverage tool (e.g., `cargo llvm-cov`,
`pytest-cov`, `c8`, `jacoco`). Exclude test helper packages and E2E suites
from the coverage denominator if they inflate the number without representing
production code quality.

**Auto-bump threshold**: a typical margin is floor + 2%. When measured coverage
exceeds this, the floor is raised to the measured value. This prevents coverage
from becoming a ceiling.

### Performance Ratchet

**What it measures**: project-defined performance metrics expressed as
quantitative ratios or absolute values (e.g., I/O efficiency ratios, p99
latency, throughput).

**How it connects to HELIX**: the Build phase enforcer requires "performance
targets met" as an exit gate. The ratchet provides the targets and prevents
them from regressing.

**Baseline awareness**: performance measurements are often hardware-dependent.
Projects with performance ratchets should cache a hardware baseline (keyed by
machine identity) and express ratchet floors as ratios to that baseline, not
as absolute values. This prevents false failures when benchmarks run on
different hardware.

**Opt-in**: not every project has meaningful performance metrics. Do not create
an empty performance ratchet fixture. Introduce it only when the project
defines concrete metrics to track.

## Integration with HELIX Phases and Actions

### Implementation Action

- **Phase 0 (Bootstrap)**: load ratchet floor fixtures from the project if they
  exist, alongside other project quality gates.
- **Phase 7 (Verification)**: run ratchet enforcement commands as part of
  verification. A ratchet failure blocks issue closure. If the measured value
  exceeds the auto-bump threshold, update the floor fixture and include the
  change in the issue commit.

### Check Action

- **Phase 2 (Artifact Health)**: report ratchet status (current measured value
  vs. floor, trend direction) as part of the artifact-health assessment. A
  ratchet that is trending downward toward the floor is a signal worth
  surfacing even if it has not yet failed.

### Reconcile-Alignment Action

- **Phase 3 (Acceptance Criteria Validation)**: the acceptance criteria
  classification that this phase already performs is the measurement input for
  the acceptance criteria ratchet. When a ratchet floor fixture exists,
  compare the current satisfaction count against the floor and flag any
  regression.
- **Phase 7 (Execution Issues)**: if a ratchet regression is detected, create
  a regression issue that references the specific criteria or metrics that
  dropped below the floor.

### Build Phase Enforcer

The existing exit requirements "coverage targets met" and "performance targets
met" are enforced through the ratchet mechanism when the project has adopted
coverage or performance ratchets. The floor fixture defines the target; the
enforcement script validates it.

### Iterate Phase

Ratchet floor trends are iterate-phase metrics. The iterate phase should:

- compare current floors to floors at the start of the cycle
- include floor deltas in the canonical iterate outputs (`metrics-dashboard`
  and `security-metrics` when relevant)
- use ratchet trends to prioritize next-cycle work (e.g., a stagnant coverage
  floor suggests the test strategy needs attention)
- feed ratchet observations back into requirements updates (feedback integration)

## Override Protocol

To intentionally lower a ratchet floor:

1. Create a tracker issue documenting:
   - which ratchet and metric are being lowered
   - the current floor and the proposed new floor
   - the justification (e.g., feature removal, architectural change,
     intentional scope reduction)
   - the acceptance criteria for restoring the previous level (or rationale
     for why restoration is not needed)
2. Lower the floor in the fixture file using the project's override mechanism
   (typically a `--force` flag on the enforcement script).
3. Commit the floor change and the issue together.

The override protocol exists to prevent ratchets from becoming immovable
obstacles. But the audit trail ensures that every regression is intentional
and traceable.

## Adopting Ratchets in a Project

A project adopts ratchets incrementally:

1. **Choose one ratchet type** to start with. Coverage is often simplest
   because it requires only a coverage tool and a single number.
2. **Create the enforcement script** that measures, compares, auto-bumps, and
   reports.
3. **Create the floor fixture file** with the initial measured value as the
   floor.
4. **Wire the script into CI** (mandatory) and pre-commit hooks (recommended
   but optional locally).
5. **Add ratchet awareness to HELIX action prompts** if using the HELIX
   workflow — the integration points are documented above.
6. **Add more ratchet types** as the project matures.

## Worked Example: Niflheim

The [niflheim](../../../) project implements all three ratchet types:

- **Acceptance criteria**: TOML manifests (`TP-*.acceptance.toml`) with
  per-criterion state tracking, enforced by three shell scripts
  (`check-acceptance-policy.sh`, `check-acceptance-traceability.sh`,
  `check-acceptance-coverage.sh`) running in CI via
  `phase0-architecture-gates.yml`.
- **Test coverage**: JSON fixture (`coverage-ratchet.json`) with a 40% floor
  and 1% tolerance, enforced by `coverage-check.sh` using `cargo llvm-cov`,
  running in CI via `coverage-gate.yml`.
- **Performance**: JSON fixture (`perf-ratchets.json`) with four I/O
  efficiency ratios against an `fio`-measured hardware baseline, enforced by
  `perf-regression-check.sh`.

The format choices reflect the problem: TOML for acceptance criteria because
they are policy documents that humans review and maintain; JSON for coverage
and performance because they are tool-generated measurement data.
