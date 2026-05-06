# Technical Context

## Background

DDx documents the bead lifecycle in the TD-036 pipeline design.

- The prose checker lives in `cli/internal/docprose`.
- The fixtures compare `file`, `line`, `rule_id`, `severity`, `rationale`, and `suggested_edit`.
- The acceptance criteria call out `TestFixtures` and `cd cli && go test ./internal/docprose/... -run TestFixtures`.

## Decision Record

ADR terminology, spec references, and list structure are deliberate here.

The checklist below is intentionally precise:

1. Keep the fixture root under `cli/internal/docprose/testdata/fixtures`.
2. Preserve the TD-036 vocabulary in technical prose.
3. Avoid flagging headings or domain terms by default.

This paragraph references DDx, bead, and Helix as project terms rather than filler.
