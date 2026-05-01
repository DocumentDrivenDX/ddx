---
name: helix-polish
description: Iteratively refine issues before implementation. Deduplication, execute-loop-ready acceptance sharpening, and dependency correction.
argument-hint: "[scope]"
disable-model-invocation: true
---

# Polish

Refine the issue queue through multiple passes before implementation begins.

"Check your issues N times, implement once."

This skill depends on shared HELIX workflow assets under `.ddx/plugins/helix/workflows/`. If the
shared library is missing, stop and report an incomplete HELIX package.

## When to Use

- After creating issues from a plan
- Whenever governing specs or designs change while open issues still exist
- When HELIX itself changes and the tracker needs to reflect the resulting
  implementation and verification work precisely
- Before starting a `helix run` cycle
- When issue quality is uneven or coverage gaps are suspected
- After importing issues from another source

## Steps

1. **Load current state** — all open issues for the scope, plus the governing
   plan document if one exists.

2. **For 4-6 rounds, refine** — each pass performs:
   - **Deduplication**: find and merge overlapping issues
   - **Coverage verification**: ensure every plan section has an issue
   - **Acceptance criteria sharpening**: replace vague criteria with testable
     statements, and require execution-ready beads to name exact commands,
     checks, or observable repository states rather than "works correctly"
   - **Dependency verification**: correct missing, circular, or incorrect deps
   - **Sizing**: split oversized issues into independently verifiable slices
   - **Label hygiene**: ensure helix, phase, kind, and area labels are correct

3. **Detect convergence** — stop when change count drops below 3 for two
   consecutive rounds.

4. **Report** — summarize modifications across rounds.

5. **Preserve executability** — ensure issues are small enough to verify,
   explicit about governing specs, and ordered so tests and safety work land
   before behavior changes where required. If a bead's acceptance text cannot
   be sharpened into explicit measurement criteria from the governing artifacts,
   flag it as not execution-ready and keep it in planning/polish rather than
   leaving hidden wrapper knowledge to decide what "done" means.

## References

- Action prompt: `.ddx/plugins/helix/workflows/actions/polish.md`
- Output template: `.ddx/plugins/helix/workflows/templates/polish-report.md`
- CLI: `helix polish [--rounds N] [scope]`
