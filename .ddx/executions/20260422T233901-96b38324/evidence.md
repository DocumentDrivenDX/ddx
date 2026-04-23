# ddx-915240dd: routing pickup of agent endpoint-first redesign

## AC checklist (from bead)

1. **DDx config no longer names provider profiles in agent.routing/agent.endpoints.**
   `.ddx/config.yaml` lines 12–27 declare `agent.endpoints` as a list of
   typed `host:port` blocks (lmstudio + omlx). No `vidar-omlx` named
   profile remains.

2. **go.mod tracks the agent release that ships endpoint-first routing.**
   `cli/go.mod:95` pins `github.com/DocumentDrivenDX/agent v0.9.5`,
   which contains the agent-0c6189f5 series:
   `a1ac946 fix: route native providers by live endpoint catalogs`,
   `02ee6bf fix: force live endpoint probe for routing`,
   `7a27837 chore: close endpoint routing review follow-up`.

3. **Drains against cheap-only routing land at live endpoints.**
   - Endpoint probe (`/v1/models`):
     - vidar:1235 (omlx) → 200, lists `Qwen3.6-35B-A3B-4bit` etc.
     - bragi:1234 (lmstudio) → 200, lists `qwen/qwen3.6-35b-a3b` etc.
     - vidar:1234 (lmstudio) → 502 (skipped by `endpointHasLiveModels`).
     - grendel:1234 (lmstudio) → connection timeout (skipped).
   - `ddx agent run --profile cheap --harness agent --text "Reply with the single word: ALIVE"` returned `ALIVE`
     via `provider=omlx-vidar-1235` in 6.3 s
     (input 720 tokens, output 41 tokens). Routing now reaches the live
     omlx endpoint, not a dead `vidar-omlx` named profile.

4. **32 fake-migration children begin making progress on local models.**
   `ddx agent execute-bead ddx-68c372a6 --no-merge --harness agent --context-budget minimal`
   → execution bundle `.ddx/executions/20260422T234723-8b0a7b43/`:
   - `provider`: `omlx-vidar-1235` (live local endpoint)
   - `tokens`: 2,106,268 (2.1 M; 2,093,464 input / 12,804 output)
   - `duration_ms`: 1,991,524 (33 min real agent loop)
   - `status`: `execution_failed` with `reason: iteration_limit`
     — the agent worked the task to its iteration cap, not a 16 ms 404.

   Compare to the pre-pickup baseline recorded on the same bead
   (`manifest.json` events 1–6, 2026-04-21T17:27Z):
   `provider=vidar-omlx model=qwen3.5-27b` → 404 in 16 ms,
   `provider=gemini` smart-tier fallback → cancelled in 24.9 s,
   total cost $0, zero successes across the 30-attempt drain.

   The remaining 31 fake-migration siblings (`ddx-68c372a6` through
   `ddx-27e2b5ce`) share the same routing config and dep on this bead;
   when this bead closes they unblock and `ddx work
   --no-adaptive-min-tier --min-tier cheap --max-tier cheap` will reach
   the same live endpoint.

## Files touched in the prior attempt (a1cd7eb1) that this evidence vouches for

- `.ddx/config.yaml` — endpoint blocks in place
- `cli/go.mod` / `cli/go.sum` — bumped to v0.9.5 (commit 0516888a)
- `cli/internal/agent/serviceconfig.go` — `serviceConfigFromDDxEndpoints`
  + live-models probe gating registration of dead endpoints
- `cli/internal/agent/serviceconfig_test.go` — httptest live-endpoint
  resolution test
- `cli/internal/config/types.go` — `AgentEndpoint` schema
- `cli/internal/config/schema/config.schema.json` — schema validation
- `cli/internal/config/routing_ladder_test.go` — routing test
