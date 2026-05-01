# omlx SSE Wire Fixtures

These fixtures drive `cli/internal/agent/omlx_e2e_test.go` — the consumer-side
end-to-end smoke test that guards `ddx work` against regressions on the
`Fizeau → vidar-omlx` dispatch path. Context: bead **ddx-bbb2d177**, which
was filed after v0.3.14 shipped the `sseCommentFilter` fix for bead
**agent-f237e07b** (upstream).

## File format

Each fixture is a JSONL file with two record kinds:

- First line: `{"kind":"meta", ...}` — describes the capture and the assertion
  shape the replay test will enforce (expected status, substrings that must or
  must not appear in the assembled content / error).
- Remaining lines: `{"kind":"frame","body":"<raw SSE body>"}` — one frame per
  record, written verbatim (including `\n\n` terminators) to the test HTTP
  response in arrival order.

Frames include both `: keep-alive` SSE comment frames (the exact shape that
triggered the v0.3.13 "unexpected end of JSON input" regression) and normal
`data: {...}` chunks.

## When to regenerate

Regenerate fixtures only when real omlx server behavior changes in a way the
existing captures no longer represent — for example, a new reasoning-phase
frame shape, a header shift, or a chunk-boundary change. Do NOT regenerate
just to refresh timestamps.

## How to regenerate

Use `scripts/capture-omlx-fixture.sh`, which wraps `ddx agent run` with
`FIZEAU_DEBUG_WIRE_STREAM_FULL=1` (shipped in Fizeau v0.3.14) so the full
stream is captured to a sidecar JSONL, then reshapes it into the fixture
format:

```bash
scripts/capture-omlx-fixture.sh \
  --endpoint http://vidar:1235/v1 \
  --model    Qwen3.6-35B-A3B-4bit \
  --prompt   "Reply with the word ready." \
  --name     happy-path \
  --status   success \
  --out      cli/internal/agent/testdata/omlx-wire/happy_path.jsonl
```

After the script writes the fixture, hand-edit the `meta.expected` block to
pin the `content_contains` / `error_contains` / `finish_reason` assertions for
the capture you just took — the script cannot infer those from the wire
bytes alone.

## Why fixtures live here instead of the upstream agent repo

The agent repo (`DocumentDrivenDX/agent`) has its own provider-level SSE
tests in `provider/openai/openai_test.go`. Those guard the SDK's handling of
SSE framing in isolation. The fixtures in *this* directory guard the DDx
consumer — `ddx work` → `Runner.RunAgent` → `agentlib.Run` — against both
SDK regressions (a bad Fizeau bump) and DDx-side regressions (a header or
request-shape change that trips omlx's parser). The two layers cover
different failure modes and both matter.
