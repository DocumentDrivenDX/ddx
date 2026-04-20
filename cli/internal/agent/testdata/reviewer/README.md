# Reviewer + closure failure-case fixtures

Byte-frozen captures of real on-disk artifacts from the 2026-04-18/20 review-malfunction incident. Consumers: ddx-f7ae036f, ddx-f8a11202, ddx-738edf47, ddx-e30e60a9.

Not synthesized. Sourced from the axon project tracker at `~/Projects/axon/` on 2026-04-20. Commit messages MUST cite the fixture the test exercises.

## Provenance

### bead-jsonl-oversized-line.jsonl

- Source: `~/Projects/axon/.ddx/beads.jsonl` (capture date 2026-04-20)
- Size: 161,121 bytes on a single JSONL line
- Pre-fix failure mode: `bufio.Scanner` with the DDx 1MB buffer succeeds, but a scanner with default 64KB buffer fails with `bufio.Scanner: token too long`. The real incident in the nexiq project reached 1.46MB on a single line; this 161KB line is the largest currently recoverable locally and still exceeds the default.
- Expected post-fix behavior (ddx-f8a11202): scanner buffer raised to ≥16MB reads the line without error; the event body is accepted or truncated-with-artifact according to the size contract.

### reviewer-stream-oversized-body.jsonl

- Source: `~/Projects/axon/.ddx/agent-logs/sessions.jsonl` (capture date 2026-04-20)
- Size: 2,688,260 bytes — the `response` field carries a full codex/claude reviewer streaming output for a single reviewer call.
- Pre-fix failure mode: the stream is written verbatim into a bead's `events[].body`, producing the oversized beads.jsonl line pattern above and corrupting downstream reads.
- Expected post-fix behavior (ddx-f8a11202): writer captures the full stream to an `.ddx/executions/<id>/reviewer-stream.log` artifact; the event body carries at most 512 bytes (verdict + first-line rationale + artifact path).

## Fixtures not yet captured

- `reviewer-stream-approve-misextracted.jsonl` — needs a session where the reviewer emitted `### Verdict: APPROVE` but DDx recorded BLOCK. Nexiq's agent-logs (the documented source) are not on local disk at capture time; axon sessions.jsonl does not obviously contain a counter-example. Deferred: a wave:2 consumer bead (ddx-f7ae036f) can capture one live once the code lands and reviewer runs against a clean bead under the current v0.7.0 stream shape.
- `bead-closed-no-evidence.jsonl` — needs axon-c5cc071a's closed-state row pre-reopen. Not found in current axon beads.jsonl and not present in axon git history for that file. Deferred: ddx-e30e60a9 can construct an equivalent via synthetic state for its invariant test, citing this README for why a historical capture was not possible.

Both deferrals are explicit — the two test-infra gaps move to the consumer beads rather than blocking fixture-capture itself.
