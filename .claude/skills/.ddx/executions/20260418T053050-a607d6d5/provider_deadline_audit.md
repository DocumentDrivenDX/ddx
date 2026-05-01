# RC4 audit: per-request HTTP deadlines on agent provider calls

**Bead:** ddx-0a651925 (child: ...-3e3913a0)
**Scope:** All main `Chat` / `ChatStream` call paths across the three providers in
`github.com/DocumentDrivenDX/agent@v0.3.14` that DDx invokes from
`cli/internal/agent/agent_runner.go`. Discovery/probe calls are noted for
completeness but are out of scope for this bead (they already install short
timeouts).

## 1. Provider-by-provider deadline audit

All findings from ripgrepping `http.Client|http.Transport|Timeout|WithTimeout|SetDeadline|ResponseHeaderTimeout`
under `~/go/pkg/mod/github.com/!document!driven!d!x/agent@v0.3.14/provider/`.

### openai (`provider/openai/openai.go`, `discovery.go`)

| Call site                                       | HTTP call owned by   | Per-request deadline? |
| ----------------------------------------------- | -------------------- | --------------------- |
| `Provider.Chat` (openai.go:183-252)             | `openai-go` SDK via `p.client.Chat.Completions.New(ctx, params)` | **No.** Client is built with `option.WithMaxRetries(0)` only. No `WithRequestTimeout`. Inherits caller ctx verbatim. |
| `Provider.ChatStream` (openai.go:336-501)       | `p.client.Chat.Completions.NewStreaming(ctx, params, ...)` | **No.** Same story. The `stream.Next()` loop reads from the underlying `http.Response.Body` for however long the socket stays open. |
| `Provider.DetectedFlavor` → `resolveProviderFlavor` (openai.go:123) | `net/http` through probe helper | 3s `context.WithTimeout`. Out of scope. |
| `DiscoverModels` (discovery.go:46)              | probe                | 5s `context.WithTimeout`. OOS. |
| `probeOpenAIServer` (discovery.go:174)          | probe                | caller-supplied timeout. OOS. |

### anthropic (`provider/anthropic/anthropic.go`)

| Call site                               | HTTP call owned by        | Per-request deadline? |
| --------------------------------------- | ------------------------- | --------------------- |
| `Provider.Chat` (anthropic.go:56-146)   | `anthropic-sdk-go` client | **No.** Built only with `option.WithMaxRetries(0)`; ctx inherited from caller. |
| `Provider.ChatStream` (anthropic.go:207-370) | streaming SDK call   | **No.** Same; streams `event := stream.Current()` for the lifetime of the socket. |

### virtual (`provider/virtual/virtual.go`)

| Call site                | HTTP call? | Deadline needed? |
| ------------------------ | ---------- | ---------------- |
| `Provider.Chat` (l.71)   | **No**     | N/A. Pure filesystem + `sleepWithContext(ctx, delayMS)`. Already ctx-cancellable. |

No streaming variant.

## 2. Why the gap hangs goroutines (failure mode)

For every main Chat/ChatStream call, the inner SDK call is `sdk.Do(ctx, req)`.
The SDK builds a `net/http` request that inherits ctx, so a caller-level cancel
eventually reaches the socket. But with RC1 fixed and RC2 added, DDx's outer
bounds are:

1. **Idle timer** — resets on every stdout/event byte. Defeated by heartbeats.
2. **Wall-clock** — 3h cap. Useful, but far too coarse for a request-level hang.

A stalled TCP socket that has delivered response headers and then stops
emitting body bytes — e.g. an OpenAI-compatible proxy that crashed mid-stream
and left its TCP connection half-open — is **not an idle timeout** at the
agent-event level because zero events ever arrived from that Chat call in the
first place. The outer wall-clock is the only thing that eventually frees the
worker, at 3h per stuck request.

Per-request deadlines shrink that window from hours (waiting for wall-clock)
to minutes (per-request cap) or sub-minute (idle-read cap inside a single
stream).

## 3. Design — wrapper, not fork

The agent library is a vendored module (`v0.3.14`). DDx cannot edit it. The
only insertion point we own is the `agentlib.Provider` that DDx constructs and
hands to `agentlib.Run` (see `agent_runner.go:94-101, 154-162`).

So the fix is a decorator: `timeoutProvider` wraps the real provider and
installs per-request deadlines transparently.

- **`Chat(ctx, ...)`** — derive `cctx, cancel := context.WithTimeout(ctx, requestTimeout)`,
  call inner, `defer cancel()`. If `cctx.Err() == DeadlineExceeded` and the
  caller ctx is still alive, return a sentinel `ErrProviderRequestTimeout`.

- **`ChatStream(ctx, ...)`** — derive a `cctx` with `requestTimeout`, call the
  inner `ChatStream(cctx, ...)`, then forward its channel through our own
  channel with a rolling idle-read timer. If the idle timer fires before the
  next delta, we cancel `cctx` (which hard-aborts the underlying HTTP read)
  and emit a `StreamDelta{Err: ErrProviderRequestTimeout}` so `consumeStream`
  unwinds with our sentinel.

- The wrapper forwards `SessionStartMetadata`, `ChatStartMetadata`, and
  `RoutingReport` to the inner provider so telemetry is preserved.

- It is installed in `buildAgentProvider` (fallback path) and
  `resolveNativeAgentProvider` (native `.agent/config.yaml` path), which are
  the only two construction sites in `agent_runner.go`.

## 4. Defaults chosen

- `DefaultProviderRequestTimeout = 15 * time.Minute` — matches the bead's
  suggested default. A single LLM call that has not completed in 15 minutes
  is almost certainly hung; local-llm flavours (omlx, lmstudio) complete in
  seconds, cloud calls in < 2 minutes, and long-context thinking calls in
  < 5 minutes.
- `DefaultProviderIdleReadTimeout = 5 * time.Minute` — the "no body bytes
  for N seconds" bound applied during streaming. 5m is loose enough to
  survive legitimate reasoning stalls (DeepSeek-R1 thinks in 30s bursts;
  Qwen3 in 10-60s) while still firing within a single turn on a truly
  stalled socket.

Both bounds are package constants today; threading them through
`Runner.Config` / catalog policy is a follow-up if needed.

## 5. Distinct failure status

`ErrProviderRequestTimeout` is a sentinel `errors.Is`-matchable error.
`RunAgent` inspects the error returned by `agentlib.Run` and, when
`errors.Is(err, ErrProviderRequestTimeout)` is true, sets
`result.Error = err.Error()` (which reads e.g. `"provider request timeout:
wall-clock 15m0s"` or `"provider request timeout: idle-read 5m0s"`).

This string then flows into `ExecuteBeadResult.Detail` through
`ExecuteBeadStatusDetail(status, reason, errMsg)` at
`execute_bead.go:742`, so the distinct phrase is visible to supervisors and
log consumers. The separator `provider request timeout:` does not collide
with the existing `"timeout after ..."` (idle) or
`"wall-clock deadline exceeded after ..."` (process wall-clock) messages,
so operators can grep unambiguously.

## 6. Out of scope

- Modifying the agent library to install client-level timeouts.
- Rewiring `AGENT_PROVIDER_REQUEST_TIMEOUT_MS` catalog/config plumbing.
- The operator-facing `ddx agent workers stop` reaper (ddx-b808df39).
- RC3 compaction-stuck faster trip (separate bead).
