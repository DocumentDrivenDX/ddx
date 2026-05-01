<bead-review>
  <bead id="ddx-d118f1bf" iter=1>
    <title>chore: track closure of upstream agent fixes on DDx side (thinking-strip, SSE-filter, preset rename)</title>
    <description>
## Context

Tracker bead for propagation of upstream ddx-agent fixes filed 2026-04-17/18.
Both fresh-eyes reviews (codex + opus, 2026-04-18) flagged the need for a
single consolidated tracker on the DDx side — this is it.

## Status table (refreshed 2026-04-19; DDx is now on agent v0.5.0)

| Upstream bead | Title                                 | Upstream status | DDx bump     | DDx commit  | DDx consumer / smoke                                                                                                       |
|---------------|---------------------------------------|-----------------|--------------|-------------|----------------------------------------------------------------------------------------------------------------------------|
| agent-92f0f324| DetectedFlavor                        | landed v0.3.12  | v0.3.11→12   | c9368312    | Covered by ddx-4817edfd (capability-gating epic; decomposed into 4 sub-beads, all closed).                                 |
| agent-767549c7| capability flags (SupportsTools)      | landed v0.3.12  | v0.3.11→12   | c9368312    | ddx-4817edfd sub-beads: context-window gating, tool-calling gating, effort/permissions gating, structured-output gating.   |
| agent-941e7e42| AGENT_DEBUG_WIRE                      | landed v0.3.12  | v0.3.11→12   | c9368312    | Used ad-hoc via scripts/capture-omlx-fixture.sh (regenerates fixtures); no CI-level regression test drives it.             |
| agent-04639431| strip thinking field on non-Anthropic | landed v0.3.13  | v0.3.12→13   | 9d057f83    | NO DDx CONSUMER NEEDED — the filter is entirely internal to agent's provider layer; DDx just sees the clean content stream.|
| agent-f237e07b| SSE comment-frame filter              | landed v0.3.14  | v0.3.13→14   | b59ebacf    | ddx-bbb2d177 smoke test landed in 1cd8e3a3 and was deleted in d5ebab2b during the v0.5.0 cutover (live-smoke equivalent now lives in the ddx-agent comparison/benchmark suite). Fixture files remain orphaned at cli/internal/agent/testdata/omlx-wire/ and should be cleaned up in a follow-up. |
| agent-a365bcf2| preset rename (cheap/default/smart)   | in_progress     | —            | —           | Blocks this bead's closure. When it lands, file a DDx migration bead for any --preset references in .ddx/config.yaml (currently at least one harness uses '--preset codex').|

## v0.5.0 cutover context (2026-04-19)

Commit d5ebab2b cut DDx over to agent v0.5.0 and dropped the local replace
directive. That post-dates the v0.3.x-era fixes this tracker was originally
scoped around. v0.5.0 contracted the public surface (provider/openai now
internal/), which is why the omlx smoke test was removed rather than
migrated. Any residual v0.3.x consumer test would be orphaned by the surface
contraction anyway — the DDx side of that consumer coverage now lives in the
upstream agent repo's benchmark suite, and DDx's own execute-bead path is
covered by the broader test set in cli/internal/agent/.

## Remaining work before this bead closes

1. Upstream agent-a365bcf2 (preset rename) lands.
2. File a DDx-side migration bead: sweep .ddx/config.yaml, library templates,
   and any hardcoded preset strings for claude/codex/cursor/agent aliases;
   swap to smart/cheap/default/minimal; optionally keep a compatibility shim
   for one release.
3. Optional (P4): delete the orphaned cli/internal/agent/testdata/omlx-wire/
   fixtures (no test consumes them post-v0.5.0) OR relocate them to the
   upstream benchmark suite.
4. Close this tracker bead.

## Non-goals

- Re-implementing the upstream fixes. They've landed.
- Tracking ddx-agent's own backlog (agent-b1c2d3e4 manifest v4 etc.). That's upstream's concern.
- Retrofitting v0.3.x-era consumer tests after the v0.5.0 surface contraction.
    </description>
    <acceptance>
1. This bead lists (and keeps current) the status of every upstream agent bead DDx filed or consumes: filed date, upstream status, DDx bump commit, any DDx-side follow-up bead(s).

2. For each landed upstream fix, a DDx-side consumer or smoke test exists that exercises the new behavior (or a tracking bead says 'no DDx consumer needed' with reasoning).

3. When agent-a365bcf2 (preset rename) lands, a migration bead is filed and this bead closes.
    </acceptance>
    <labels>ddx, phase:build, kind:chore, area:agent, area:vendoring</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1f81e309cad5cbd759429cdfe99004ae8c076e60">

  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-d118f1bf iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>
