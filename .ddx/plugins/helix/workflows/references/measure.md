# Reference: Measure Phase

The measure phase verifies results against a bead's acceptance criteria and
records evidence on the bead. It runs as an embedded phase within every action
and is also available as a standalone command.

## What Measure Checks

Measure runs these verification steps in order:

### 1. Acceptance Criteria

For each criterion in the bead's acceptance text:

- Determine the verification method (test command, file existence, manual check).
- Run the verification.
- Record pass/fail with evidence (command output, file path, test name).

If the bead has a `spec-id`, check for an acceptance manifest (e.g.,
`TP-SD-010.acceptance.toml`) and verify against it.

### 2. Concern-Declared Quality Gates

Load active concerns per `.ddx/plugins/helix/workflows/references/concern-resolution.md`, filtered
by the bead's area labels. For each matched concern, run the quality gates
declared in its `practices.md` under `## Quality Gates`:

| Concern | Typical gates |
|---------|--------------|
| `rust-cargo` | `cargo clippy`, `cargo fmt --check`, `cargo deny check advisories` |
| `typescript-bun` | `biome check`, `bun test` |
| `python-uv` | `ruff check`, `ruff format --check`, `pyright` |
| `go-std` | `go vet`, `golangci-lint run`, `govulncheck` |
| `security-owasp` | Per-stack dependency audit command |

Scope gate runs to changed packages, not the full workspace. Use project
overrides from `docs/helix/01-frame/concerns.md` when they specify alternative
commands.

### 3. Ratchet Enforcement

If the project has adopted quality ratchets (see `.ddx/plugins/helix/workflows/ratchets.md`):

- Load ratchet floor fixtures.
- Run the enforcement command for each applicable ratchet.
- Record measured value vs. floor.
- If auto-bump is triggered, include the updated floor fixture in the results.

### 4. Concern Propagation Check

Verify that the bead's context digest includes all active concerns for its
area scope. If the digest is stale (concerns changed since assembly), flag it.

## Recording Results

Measurement results are recorded on the bead using structured notes:

```bash
ddx bead update <id> --notes "$(cat <<'EOF'
<measure-results>
  <timestamp>2026-04-06T14:30:00Z</timestamp>
  <status>PASS|FAIL|PARTIAL</status>
  <acceptance>
    <criterion name="..." status="pass|fail" evidence="..."/>
  </acceptance>
  <gates>
    <gate concern="rust-cargo" command="cargo clippy" status="pass|fail"/>
  </gates>
  <ratchets>
    <ratchet name="coverage" floor="80" measured="83" status="pass"/>
  </ratchets>
</measure-results>
EOF
)"
```

The `<measure-results>` block is machine-parseable. `helix report` reads it
when analyzing results.

## Standalone Usage

```bash
helix measure <bead-id>       # measure a specific bead
helix measure <scope>         # measure all beads in scope
helix measure --rerun <id>    # re-run measurement (e.g., after a fix)
```

Standalone `helix measure` is useful for:

- Re-measuring after a fix without re-running the full action.
- Batch measurement across a scope (e.g., verifying all beads in an epic).
- CI/CD integration — run measurement as a gate.

## Embedded Usage

When measure runs as an embedded phase within an action, it follows the same
steps but uses the action's already-loaded context (concerns, principles,
ratchet fixtures) rather than re-loading from scratch.

The embedded measure phase is typically Phase N+1, immediately after the
action's main execution phases and before the report phase.

## Measure Status

Measure produces one of three statuses:

- `PASS`: All acceptance criteria satisfied, all gates passed, all ratchets
  within tolerance.
- `FAIL`: One or more acceptance criteria failed or a gate/ratchet failed.
- `PARTIAL`: Some criteria passed, some could not be verified (e.g., manual
  check required, external dependency unavailable).

## Output

```
MEASURE_STATUS: PASS|FAIL|PARTIAL
CRITERIA_TOTAL: N
CRITERIA_PASSED: N
CRITERIA_FAILED: N
GATES_RUN: N
GATES_PASSED: N
RATCHETS_CHECKED: N
RATCHETS_PASSED: N
```
