---
ddx:
  id: TP-007
  depends_on:
    - FEAT-001
    - SD-007
---
# Test Plan: E2E Smoke Tests for Core CLI Journey

## Objective

Validate that the install-to-use journey works end-to-end against the real
binary. These tests catch integration failures that unit tests miss —
broken command wiring, missing config defaults, initialization side effects.

## Scope

The smoke tests cover the core onboarding path a new user follows:

1. `ddx init` in a fresh git repo
2. `ddx list` shows document categories
3. `ddx doctor` reports healthy status
4. `ddx persona list` shows available personas
5. `ddx persona bind` saves a binding
6. `ddx bead create` creates a work item
7. `ddx bead list` shows the created bead

## Test Environment

- Fresh temp directory with `git init`
- Built `ddx` binary (not `go run`)
- No pre-existing `.ddx/` directory
- Controlled `HOME` to isolate from user config

## Test Cases

### TC-001: Init creates expected structure
**Given** a fresh git repo with no `.ddx/` directory
**When** `ddx init` runs
**Then** exit code is 0, `.ddx/config.yaml` exists, `.ddx/library/` exists

### TC-002: List shows categories after init
**Given** a repo with `ddx init` completed
**When** `ddx list` runs
**Then** exit code is 0, output contains "prompts", "personas", "templates"

### TC-003: Doctor reports healthy
**Given** a repo with `ddx init` completed
**When** `ddx doctor` runs
**Then** exit code is 0, output does not contain "ERROR" or "FAIL"

### TC-004: Persona list shows personas
**Given** a repo with `ddx init` completed
**When** `ddx persona list` runs
**Then** exit code is 0, output lists at least one persona

### TC-005: Persona bind saves binding
**Given** a repo with `ddx init` completed
**When** `ddx persona bind code-reviewer strict-code-reviewer` runs
**Then** exit code is 0, `.ddx.yml` contains the binding

### TC-006: Bead create produces a bead
**Given** a repo with `ddx init` completed
**When** `ddx bead create --title "Smoke test" --type task` runs
**Then** exit code is 0, output contains the bead ID

### TC-007: Bead list shows the created bead
**Given** a bead was created in TC-006
**When** `ddx bead list` runs
**Then** exit code is 0, output contains "Smoke test"

## Implementation

Tests live in `cli/cmd/e2e_smoke_test.go` following the existing acceptance
test pattern. Each test case is a subtest within a single `TestE2ESmokeJourney`
function that shares the temp directory (sequential execution, ordered steps).

Add to lefthook's `ci` group so tests run both locally and in CI.

## Pass Criteria

All 7 test cases pass on Linux (CI) and developer machines (macOS/Linux).
