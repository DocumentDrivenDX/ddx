<bead-review>
  <bead id="ddx-29058e2a" iter=1>
    <title>agent: unify ExecuteLoopSpec as single source of truth across cobra/HTTP/server/worker layers</title>
    <description>
The bug at ddx-29058e2a (5 CLI flags silently dropped on the server path) is a symptom of a deeper architectural issue: the ExecuteLoopSpec is defined as parallel structs that must be hand-maintained in lockstep. Adding any flag requires editing 5+ places, and forgetting one is silent.

CURRENT STATE (the smell)

Each flag traverses 6 boundaries:

1. cobra extract: cmd.Flags().GetX("flag") → local var (cli/cmd/agent_cmd.go:1573-1611)
2. positional param: passed to executeLoopWithServer / runExecuteLoopLocal (cli/cmd/agent_cmd.go:1627, ~1941 signature)
3. loose JSON map: workerSpec map[string]interface{} (cli/cmd/agent_cmd.go:~1948)
4. server request struct: hand-maintained in handleStartExecuteLoopWorker (cli/internal/server/server.go:2246-2260)
5. ExecuteLoopWorkerSpec literal: server.go:2289-2303 → cli/internal/server/workers.go:25-49
6. runWorker per-field consumption: cli/internal/server/workers.go ~656

ALSO IN SCOPE — the GraphQL parallel path (per codex review):
- workerDispatchAdapter at cli/internal/server/graphql_adapters.go:62-75, 123-137 has its own request struct AND its own StartExecuteLoop literal — same parallel-struct smell on a different transport. Unification must DELETE this too, otherwise the refactor fixes REST and leaves GraphQL re-broken.

Two of those (steps 4 and 5) are typed but maintained in parallel. The 5 dropped flags (opaque_passthrough, max_cost, request_timeout, min_power, max_power) fell out exactly there. PLUS FromRev is also dropped on the server path (cobra flag at agent_cmd.go:1560 + work.go:58, consumed locally at agent_cmd.go:1753 ExecuteBeadRuntime{FromRev: fromRev}, never reaches the server). The bug fix would be local — add fields in 5 places — but the ROOT CAUSE recurs the next time a flag is added.

DESIGN — single canonical ExecuteLoopSpec struct

Define one struct, used at every layer:

  cobra RunE → parseExecuteLoopSpec(cmd) (ExecuteLoopSpec, DispatchOptions)
                ↓
       (--local) runExecuteLoopInline(spec)
                ↓
       (default) httpSubmitExecuteLoop(spec)  ── JSON ──▶ server.handleStartExecuteLoopWorker
                                                   ↓ (decode INTO THE SAME struct, then Validate + ApplyDefaults)
                                                startExecuteLoop(spec)
                                                   ↓ (persist as spec.json — SAME struct, fully-resolved)
                                                runWorker(spec)

Properties:
- One struct definition. JSON tags drive both wire format and disk persistence.
- Cobra binding is the only per-flag code (irreducible — cobra needs to know each flag).
- --local and server paths consume the same struct; behavior parity is structural, not asserted by tests.
- Adding a flag = (a) one struct field + (b) one cobra binding. Drop is impossible by construction — the JSON tag round-trips through wire and persistence.
- DispatchOptions is a SEPARATE small struct holding control-plane flags (--local, --json) that are NOT execution-spec fields and must NOT be persisted on disk.

CRITICAL CONTRACTS (per codex review)

1. Wire format for time.Duration: Go's default time.Duration JSON marshaling produces nanoseconds, but current code sends poll_interval as a string (agent_cmd.go:1969-1971). The unified spec MUST commit to one format — either custom Duration type with string MarshalJSON, or canonical numeric. Pick string (operator-readable spec.json wins) and add UnmarshalJSON tolerating both.

2. Validate + ApplyDefaults lifecycle:
   - parseExecuteLoopSpec returns spec + DispatchOptions, with cobra defaults applied
   - Spec.Validate() is called BEFORE HTTP submit (client-side guard)
   - server.handleStartExecuteLoopWorker decodes → calls Spec.ApplyDefaults() → Spec.Validate() BEFORE StartExecuteLoop (server cannot trust the wire)
   - GraphQL workerDispatchAdapter goes through THE SAME StartExecuteLoop entry point so Validate + ApplyDefaults are guaranteed
   - spec.json on disk is fully-resolved (no implicit defaults at runWorker time)

3. Forward-compat stance: ddx is single-repo, single-version client/server today. Add SpecVersion int field; server returns 400 with capability list if SpecVersion &gt; supported. Newer-server-older-client: existing fields unchanged, new fields ignored on both sides (the current default). Make this explicit in the AC; do not rely on json's silent unknown-field tolerance as the only contract.

4. Backward-compat for old persisted specs: existing spec.json files in .ddx/workers/ have legacy fields (e.g., min_tier/max_tier in older specs at .ddx/workers/worker-20260422T083934-f77c/spec.json:5-6). UnmarshalJSON must tolerate unknown fields when LOADING from disk (json.Decoder default behavior is fine here; do NOT enable DisallowUnknownFields for disk reads).

LOCATION

Likely cli/internal/agent/spec.go or cli/internal/agent/executeloop/spec.go (a small focused package). Importable from both cli/cmd/ and cli/internal/server/ without import cycles. NOTE: REACH-PROTO (ddx-a946c744) needs to know about this new package as an entry-root for the production-reachability checker.

NOT IN SCOPE

- Other commands (ddx agent run, ddx try, ddx run, MCP agent dispatch, ddx work passthrough variants) — same smell may exist; covered by the sister audit bead ddx-fb290074.
- Reworking client→server transport (HTTP/JSON stays as-is).
- Changing validation logic itself — only ensuring it lives on the spec so both paths apply it.
- Renaming flags or reshaping CLI surface.
- Reshaping LabelFilter from string to []string — that is a picker semantics change, owned by ddx-9d55601f. Keep LabelFilter as string to keep ddx-9d55601f landing in parallel.

USER-OBSERVED SYMPTOM (originally what filed this bead)
- ddx work --harness claude works under --local but fails when going through the server.
- Root cause: opaque_passthrough was dropped at the server's request struct. With OpaquePassthrough lost, the server-spawned worker validates the harness against its provider list (which rejects 'claude') instead of treating it as opaque-passthrough.
- The unification refactor solves this by construction — there's no second struct to omit OpaquePassthrough from. NOTE: OpaquePassthrough has TWO skip points to verify (cli/internal/server/workers.go:314-317 AND :752-773), not one.

ABSORBS THE NARROW FIX
The 5 originally-listed dropped flags + FromRev all become fields on the unified spec. They cannot be dropped after this refactor. The refactor IS the fix.

SEQUENCING WITH IN-FLIGHT BEADS

- ddx-dc157075 (worker stay-alive): touches execute_bead_loop.go's Run loop. Should land BEFORE this OR be tightly coordinated. If unification lands first, dc157075 redoes defaulting + persistence. Recommend dc157075 first.
- ddx-9d55601f (picker priority bug): does NOT block on this. Keep LabelFilter type stable. Picker fix lands in parallel.
- ddx-a946c744 (REACH-PROTO): needs to know about new cli/internal/agent/executeloop/ package as entry root.
    </description>
    <acceptance>
1. Single struct ExecuteLoopSpec in cli/internal/agent/ (or sub-package) with JSON tags. Includes ALL fields currently distributed across cobra extracts, server request struct, ExecuteLoopWorkerSpec, and runWorker consumption: ProjectRoot, Harness, Model, Profile, Provider, ModelRef, Effort, LabelFilter, Once, PollInterval, NoReview, ReviewHarness, ReviewModel, OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev, plus SpecVersion int. Audit current code: any field in cobra/server/spec/runWorker NOT in this list = bug, must add.

2. SEPARATE DispatchOptions struct holds control-plane flags (--local, --json) that must NOT appear on ExecuteLoopSpec or be persisted to spec.json. parseExecuteLoopSpec returns (ExecuteLoopSpec, DispatchOptions, error) — single cobra→spec conversion; called only from cobra RunE; never re-implemented.

3. Spec.Validate() and Spec.ApplyDefaults() exist as methods. Lifecycle:
   (a) parseExecuteLoopSpec → cobra defaults applied via cobra binding
   (b) client-side: Validate() before HTTP submit
   (c) server-side: decode → ApplyDefaults() → Validate() BEFORE StartExecuteLoop
   (d) GraphQL workerDispatchAdapter goes through the SAME StartExecuteLoop (no parallel construction)
   (e) spec.json on disk is fully-resolved (ApplyDefaults already ran)

4. JSON wire format for time.Duration uses string representation (e.g. "30s", "5m"). UnmarshalJSON tolerates both string and numeric (nanoseconds) for backward-compat. Custom Duration type or json.RawMessage→time.ParseDuration acceptable.

5. cli/cmd/agent_cmd.go execute-loop / work RunE: constructs (spec, dispatch) via parseExecuteLoopSpec; dispatches: dispatch.Local → runExecuteLoopInline(spec); else httpSubmitExecuteLoop(spec). NO positional-param flag passing remains.

6. httpSubmitExecuteLoop marshals the spec directly as the HTTP request body (json.Marshal(spec)). No intermediate map[string]interface{}.

7. Server handleStartExecuteLoopWorker decodes directly INTO ExecuteLoopSpec (no parallel request struct). The previously hand-maintained struct in server.go:2246-2260 is DELETED. ApplyDefaults + Validate run before StartExecuteLoop.

8. GraphQL workerDispatchAdapter (cli/internal/server/graphql_adapters.go:62-75, 123-137) DELETES its parallel request struct and parallel StartExecuteLoop literal. Goes through the same server-side ApplyDefaults + Validate + StartExecuteLoop path as the REST handler.

9. ExecuteLoopWorkerSpec in workers.go is REPLACED by (or aliased to) ExecuteLoopSpec. spec.json on disk uses the unified struct.

10. runWorker(spec) consumes spec.MaxCostUSD (with default fallback when zero), spec.RequestTimeout, spec.MinPower, spec.MaxPower, spec.OpaquePassthrough, spec.FromRev — no hardcoded escalation.DefaultMaxCostUSD anywhere except the fallback case.

11. Reflection-backed round-trip test (TestExecuteLoopSpec_RoundTripsAllFields_Reflection): uses reflect over ExecuteLoopSpec's exported fields; for each, sets a non-zero/non-default value, marshals, unmarshals on the server side, asserts the value survives into spec.json AND into the runWorker-visible spec. Adding a future field = automatic test coverage (no row-by-row maintenance). Test FAILS if any exported field is missing source/sink coverage.

12. Specific OpaquePassthrough assertion: server-submitted ddx work --harness claude persists opaque_passthrough:true and skips BOTH validation gates (cli/internal/server/workers.go:314-317 AND :752-773). Test names both line ranges.

13. Backward compatibility: existing spec.json files in .ddx/workers/ continue to load. Specifically: legacy fields min_tier, max_tier, and any other historical fields are tolerated and ignored (json.Decoder default behavior; do NOT enable DisallowUnknownFields for disk reads). Spot-check: load .ddx/workers/worker-20260422T083934-f77c/spec.json without error.

14. Forward compatibility: SpecVersion int field added. Server rejects with 400 + capability list when client SpecVersion &gt; server-supported. Same client/server version always works. Document the stance in code comment.

15. Regression test: --local path is unchanged in behavior. Existing local-path tests pass.

16. Manual verification: `ddx work --harness claude` (no --local) on a small queue completes at least one bead end-to-end without harness-rejection error.

17. cd cli &amp;&amp; go test ./cmd/... ./internal/agent/... ./internal/server/... green; lefthook pre-commit passes.

DEPENDENCIES
- Recommend landing AFTER ddx-dc157075 (worker stay-alive). If landing in parallel, coordinate execute_bead_loop.go merges.
- ddx-9d55601f (picker) is independent — LabelFilter stays string.
- REACH-PROTO (ddx-a946c744): new package added as entry-root.
    </acceptance>
    <notes>
decomposed into ddx-1a9cc01f (canonical ExecuteLoopSpec package), ddx-76cf71f4 (cobra parse/DispatchOptions), ddx-da9e9491 (REST handler decode/validation), ddx-ce1d6309 (GraphQL dispatch unification), ddx-89a9c305 (worker spec/persistence/runtime migration), and ddx-16722d4e (REACH-PROTO entry-root follow-through).
    </notes>
    <labels>phase:2, area:agent, area:server, kind:refactor, prevention, triage:needs-investigation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T021759-c712d0e8/manifest.json</file>
    <file>.ddx/executions/20260504T021759-c712d0e8/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0e497b4040a88f26c58a99dd272bec5992b9f23e">
diff --git a/.ddx/executions/20260504T021759-c712d0e8/manifest.json b/.ddx/executions/20260504T021759-c712d0e8/manifest.json
new file mode 100644
index 00000000..42303447
--- /dev/null
+++ b/.ddx/executions/20260504T021759-c712d0e8/manifest.json
@@ -0,0 +1,847 @@
+{
+  "attempt_id": "20260504T021759-c712d0e8",
+  "bead_id": "ddx-29058e2a",
+  "base_rev": "b21843270d688978cf5d521d4dcba4b4d81e7adf",
+  "created_at": "2026-05-04T02:18:00.787468288Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-29058e2a",
+    "title": "agent: unify ExecuteLoopSpec as single source of truth across cobra/HTTP/server/worker layers",
+    "description": "The bug at ddx-29058e2a (5 CLI flags silently dropped on the server path) is a symptom of a deeper architectural issue: the ExecuteLoopSpec is defined as parallel structs that must be hand-maintained in lockstep. Adding any flag requires editing 5+ places, and forgetting one is silent.\n\nCURRENT STATE (the smell)\n\nEach flag traverses 6 boundaries:\n\n1. cobra extract: cmd.Flags().GetX(\"flag\") → local var (cli/cmd/agent_cmd.go:1573-1611)\n2. positional param: passed to executeLoopWithServer / runExecuteLoopLocal (cli/cmd/agent_cmd.go:1627, ~1941 signature)\n3. loose JSON map: workerSpec map[string]interface{} (cli/cmd/agent_cmd.go:~1948)\n4. server request struct: hand-maintained in handleStartExecuteLoopWorker (cli/internal/server/server.go:2246-2260)\n5. ExecuteLoopWorkerSpec literal: server.go:2289-2303 → cli/internal/server/workers.go:25-49\n6. runWorker per-field consumption: cli/internal/server/workers.go ~656\n\nALSO IN SCOPE — the GraphQL parallel path (per codex review):\n- workerDispatchAdapter at cli/internal/server/graphql_adapters.go:62-75, 123-137 has its own request struct AND its own StartExecuteLoop literal — same parallel-struct smell on a different transport. Unification must DELETE this too, otherwise the refactor fixes REST and leaves GraphQL re-broken.\n\nTwo of those (steps 4 and 5) are typed but maintained in parallel. The 5 dropped flags (opaque_passthrough, max_cost, request_timeout, min_power, max_power) fell out exactly there. PLUS FromRev is also dropped on the server path (cobra flag at agent_cmd.go:1560 + work.go:58, consumed locally at agent_cmd.go:1753 ExecuteBeadRuntime{FromRev: fromRev}, never reaches the server). The bug fix would be local — add fields in 5 places — but the ROOT CAUSE recurs the next time a flag is added.\n\nDESIGN — single canonical ExecuteLoopSpec struct\n\nDefine one struct, used at every layer:\n\n  cobra RunE → parseExecuteLoopSpec(cmd) (ExecuteLoopSpec, DispatchOptions)\n                ↓\n       (--local) runExecuteLoopInline(spec)\n                ↓\n       (default) httpSubmitExecuteLoop(spec)  ── JSON ──▶ server.handleStartExecuteLoopWorker\n                                                   ↓ (decode INTO THE SAME struct, then Validate + ApplyDefaults)\n                                                startExecuteLoop(spec)\n                                                   ↓ (persist as spec.json — SAME struct, fully-resolved)\n                                                runWorker(spec)\n\nProperties:\n- One struct definition. JSON tags drive both wire format and disk persistence.\n- Cobra binding is the only per-flag code (irreducible — cobra needs to know each flag).\n- --local and server paths consume the same struct; behavior parity is structural, not asserted by tests.\n- Adding a flag = (a) one struct field + (b) one cobra binding. Drop is impossible by construction — the JSON tag round-trips through wire and persistence.\n- DispatchOptions is a SEPARATE small struct holding control-plane flags (--local, --json) that are NOT execution-spec fields and must NOT be persisted on disk.\n\nCRITICAL CONTRACTS (per codex review)\n\n1. Wire format for time.Duration: Go's default time.Duration JSON marshaling produces nanoseconds, but current code sends poll_interval as a string (agent_cmd.go:1969-1971). The unified spec MUST commit to one format — either custom Duration type with string MarshalJSON, or canonical numeric. Pick string (operator-readable spec.json wins) and add UnmarshalJSON tolerating both.\n\n2. Validate + ApplyDefaults lifecycle:\n   - parseExecuteLoopSpec returns spec + DispatchOptions, with cobra defaults applied\n   - Spec.Validate() is called BEFORE HTTP submit (client-side guard)\n   - server.handleStartExecuteLoopWorker decodes → calls Spec.ApplyDefaults() → Spec.Validate() BEFORE StartExecuteLoop (server cannot trust the wire)\n   - GraphQL workerDispatchAdapter goes through THE SAME StartExecuteLoop entry point so Validate + ApplyDefaults are guaranteed\n   - spec.json on disk is fully-resolved (no implicit defaults at runWorker time)\n\n3. Forward-compat stance: ddx is single-repo, single-version client/server today. Add SpecVersion int field; server returns 400 with capability list if SpecVersion \u003e supported. Newer-server-older-client: existing fields unchanged, new fields ignored on both sides (the current default). Make this explicit in the AC; do not rely on json's silent unknown-field tolerance as the only contract.\n\n4. Backward-compat for old persisted specs: existing spec.json files in .ddx/workers/ have legacy fields (e.g., min_tier/max_tier in older specs at .ddx/workers/worker-20260422T083934-f77c/spec.json:5-6). UnmarshalJSON must tolerate unknown fields when LOADING from disk (json.Decoder default behavior is fine here; do NOT enable DisallowUnknownFields for disk reads).\n\nLOCATION\n\nLikely cli/internal/agent/spec.go or cli/internal/agent/executeloop/spec.go (a small focused package). Importable from both cli/cmd/ and cli/internal/server/ without import cycles. NOTE: REACH-PROTO (ddx-a946c744) needs to know about this new package as an entry-root for the production-reachability checker.\n\nNOT IN SCOPE\n\n- Other commands (ddx agent run, ddx try, ddx run, MCP agent dispatch, ddx work passthrough variants) — same smell may exist; covered by the sister audit bead ddx-fb290074.\n- Reworking client→server transport (HTTP/JSON stays as-is).\n- Changing validation logic itself — only ensuring it lives on the spec so both paths apply it.\n- Renaming flags or reshaping CLI surface.\n- Reshaping LabelFilter from string to []string — that is a picker semantics change, owned by ddx-9d55601f. Keep LabelFilter as string to keep ddx-9d55601f landing in parallel.\n\nUSER-OBSERVED SYMPTOM (originally what filed this bead)\n- ddx work --harness claude works under --local but fails when going through the server.\n- Root cause: opaque_passthrough was dropped at the server's request struct. With OpaquePassthrough lost, the server-spawned worker validates the harness against its provider list (which rejects 'claude') instead of treating it as opaque-passthrough.\n- The unification refactor solves this by construction — there's no second struct to omit OpaquePassthrough from. NOTE: OpaquePassthrough has TWO skip points to verify (cli/internal/server/workers.go:314-317 AND :752-773), not one.\n\nABSORBS THE NARROW FIX\nThe 5 originally-listed dropped flags + FromRev all become fields on the unified spec. They cannot be dropped after this refactor. The refactor IS the fix.\n\nSEQUENCING WITH IN-FLIGHT BEADS\n\n- ddx-dc157075 (worker stay-alive): touches execute_bead_loop.go's Run loop. Should land BEFORE this OR be tightly coordinated. If unification lands first, dc157075 redoes defaulting + persistence. Recommend dc157075 first.\n- ddx-9d55601f (picker priority bug): does NOT block on this. Keep LabelFilter type stable. Picker fix lands in parallel.\n- ddx-a946c744 (REACH-PROTO): needs to know about new cli/internal/agent/executeloop/ package as entry root.",
+    "acceptance": "1. Single struct ExecuteLoopSpec in cli/internal/agent/ (or sub-package) with JSON tags. Includes ALL fields currently distributed across cobra extracts, server request struct, ExecuteLoopWorkerSpec, and runWorker consumption: ProjectRoot, Harness, Model, Profile, Provider, ModelRef, Effort, LabelFilter, Once, PollInterval, NoReview, ReviewHarness, ReviewModel, OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev, plus SpecVersion int. Audit current code: any field in cobra/server/spec/runWorker NOT in this list = bug, must add.\n\n2. SEPARATE DispatchOptions struct holds control-plane flags (--local, --json) that must NOT appear on ExecuteLoopSpec or be persisted to spec.json. parseExecuteLoopSpec returns (ExecuteLoopSpec, DispatchOptions, error) — single cobra→spec conversion; called only from cobra RunE; never re-implemented.\n\n3. Spec.Validate() and Spec.ApplyDefaults() exist as methods. Lifecycle:\n   (a) parseExecuteLoopSpec → cobra defaults applied via cobra binding\n   (b) client-side: Validate() before HTTP submit\n   (c) server-side: decode → ApplyDefaults() → Validate() BEFORE StartExecuteLoop\n   (d) GraphQL workerDispatchAdapter goes through the SAME StartExecuteLoop (no parallel construction)\n   (e) spec.json on disk is fully-resolved (ApplyDefaults already ran)\n\n4. JSON wire format for time.Duration uses string representation (e.g. \"30s\", \"5m\"). UnmarshalJSON tolerates both string and numeric (nanoseconds) for backward-compat. Custom Duration type or json.RawMessage→time.ParseDuration acceptable.\n\n5. cli/cmd/agent_cmd.go execute-loop / work RunE: constructs (spec, dispatch) via parseExecuteLoopSpec; dispatches: dispatch.Local → runExecuteLoopInline(spec); else httpSubmitExecuteLoop(spec). NO positional-param flag passing remains.\n\n6. httpSubmitExecuteLoop marshals the spec directly as the HTTP request body (json.Marshal(spec)). No intermediate map[string]interface{}.\n\n7. Server handleStartExecuteLoopWorker decodes directly INTO ExecuteLoopSpec (no parallel request struct). The previously hand-maintained struct in server.go:2246-2260 is DELETED. ApplyDefaults + Validate run before StartExecuteLoop.\n\n8. GraphQL workerDispatchAdapter (cli/internal/server/graphql_adapters.go:62-75, 123-137) DELETES its parallel request struct and parallel StartExecuteLoop literal. Goes through the same server-side ApplyDefaults + Validate + StartExecuteLoop path as the REST handler.\n\n9. ExecuteLoopWorkerSpec in workers.go is REPLACED by (or aliased to) ExecuteLoopSpec. spec.json on disk uses the unified struct.\n\n10. runWorker(spec) consumes spec.MaxCostUSD (with default fallback when zero), spec.RequestTimeout, spec.MinPower, spec.MaxPower, spec.OpaquePassthrough, spec.FromRev — no hardcoded escalation.DefaultMaxCostUSD anywhere except the fallback case.\n\n11. Reflection-backed round-trip test (TestExecuteLoopSpec_RoundTripsAllFields_Reflection): uses reflect over ExecuteLoopSpec's exported fields; for each, sets a non-zero/non-default value, marshals, unmarshals on the server side, asserts the value survives into spec.json AND into the runWorker-visible spec. Adding a future field = automatic test coverage (no row-by-row maintenance). Test FAILS if any exported field is missing source/sink coverage.\n\n12. Specific OpaquePassthrough assertion: server-submitted ddx work --harness claude persists opaque_passthrough:true and skips BOTH validation gates (cli/internal/server/workers.go:314-317 AND :752-773). Test names both line ranges.\n\n13. Backward compatibility: existing spec.json files in .ddx/workers/ continue to load. Specifically: legacy fields min_tier, max_tier, and any other historical fields are tolerated and ignored (json.Decoder default behavior; do NOT enable DisallowUnknownFields for disk reads). Spot-check: load .ddx/workers/worker-20260422T083934-f77c/spec.json without error.\n\n14. Forward compatibility: SpecVersion int field added. Server rejects with 400 + capability list when client SpecVersion \u003e server-supported. Same client/server version always works. Document the stance in code comment.\n\n15. Regression test: --local path is unchanged in behavior. Existing local-path tests pass.\n\n16. Manual verification: `ddx work --harness claude` (no --local) on a small queue completes at least one bead end-to-end without harness-rejection error.\n\n17. cd cli \u0026\u0026 go test ./cmd/... ./internal/agent/... ./internal/server/... green; lefthook pre-commit passes.\n\nDEPENDENCIES\n- Recommend landing AFTER ddx-dc157075 (worker stay-alive). If landing in parallel, coordinate execute_bead_loop.go merges.\n- ddx-9d55601f (picker) is independent — LabelFilter stays string.\n- REACH-PROTO (ddx-a946c744): new package added as entry-root.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:server",
+      "kind:refactor",
+      "prevention",
+      "triage:needs-investigation"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-04T02:17:59Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2986569",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-02T20:54:37.989096911Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "exit status 1\nresult_rev=dd174d4ac0529ac46a42985281f3113fe54efa51\nbase_rev=dd174d4ac0529ac46a42985281f3113fe54efa51\nretry_after=2026-05-03T02:54:38Z",
+          "created_at": "2026-05-02T20:54:38.513521822Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-03T03:09:52.257218554Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260503T025809-45a220f7\",\"harness\":\"claude\",\"input_tokens\":0,\"output_tokens\":0,\"total_tokens\":0,\"cost_usd\":2.81355875,\"duration_ms\":689043,\"exit_code\":1}",
+          "created_at": "2026-05-03T03:09:52.275641898Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=0 cost_usd=2.8136"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=e24ac3d8e7fe88b3f56cd9242cb058b2e754c4d7\nbase_rev=e24ac3d8e7fe88b3f56cd9242cb058b2e754c4d7\nretry_after=2026-05-03T09:09:52Z",
+          "created_at": "2026-05-03T03:09:52.392741346Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:10:00.001785988Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:10:39.181501294Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:11:25.145435136Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:12:20.353187968Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:13:22.198426726Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:14:35.613102353Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:15:58.103090814Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:17:34.175112124Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:19:28.626375351Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:21:25.569710852Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:23:22.410018474Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:25:19.278700375Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:27:16.274674365Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:29:13.429313185Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:31:10.566697276Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:33:07.419235747Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:35:04.184061705Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:37:01.152397521Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:38:57.951942375Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:40:54.855766742Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:42:51.688792235Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:44:48.262699727Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:46:45.175561139Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:48:42.107359067Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:50:39.13371681Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:52:36.122180665Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:54:33.268209484Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:56:30.176800846Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T09:58:27.281849582Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:00:24.677836218Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:02:21.847585994Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:04:19.009708632Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:06:15.814809043Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:08:12.962504824Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:10:10.20966824Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:12:07.197964928Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:14:04.399381404Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:16:01.445188882Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:17:58.491288695Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:19:55.551324053Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:21:52.543597085Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:23:49.84760729Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:25:47.286327845Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:27:44.279556457Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:29:41.459843579Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:31:38.667246979Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:33:35.783842586Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:35:33.056449948Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:37:30.45443982Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:39:27.969343436Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:41:25.374705633Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:43:22.817351114Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:45:20.179895331Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:47:17.334717113Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:49:14.856839077Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:51:12.323604286Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:53:07.379507982Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:55:05.066540416Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:57:02.392575439Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T10:58:59.514943821Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:00:57.016005902Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:02:56.735570252Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T11:04:56.32568734Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-03T11:09:20.763694982Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260503T110552-7129c1c3\",\"harness\":\"claude\",\"input_tokens\":17,\"output_tokens\":8486,\"total_tokens\":8503,\"cost_usd\":0.86102725,\"duration_ms\":206193,\"exit_code\":0}",
+          "created_at": "2026-05-03T11:09:20.805718788Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=8503 cost_usd=0.8610"
+        },
+        {
+          "actor": "erik",
+          "body": "Architectural conflict with open bead ddx-eccc6efb (ADR-022 step 7), plus the bead materially exceeds the size-check thresholds. Human triage required to resolve the design conflict before any unification work commits. ## Summary The bead asks to UNIFY the parallel ExecuteLoopWorkerSpec / cobra-flag / HTTP-request structs into one canonical `ExecuteLoopSpec` struct used at every layer. But the open ADR-022 step 7 epic (ddx-eccc6efb, status=open) takes the opposite direction: DELETE `ExecuteLoopWorkerSpec` entirely and replace the in-process server-spawn path with `exec ddx work` (subprocess), at which point there are no parallel structs because the server stops marshalling a spec at all — it just builds an argv vector. These are mutually-exclusive architectural directions for the same code. Landing this bead's unification would either: (a) be partially undone when ADR-022 step 7 deletes the unified struct, or (b) lock in the unified-struct design and require ADR-022 step 7 to be re-scoped or abandoned. The bead's \"SEQUENCING WITH IN-FLIGHT BEADS\" section names dc157075, 9d55601f, and a946c744, but is silent on ddx-eccc6efb / ADR-022 step 7. That gap is the load-bearing decision this bead does not make. ## Evidence 1. `cli/internal/server/workers.go:25-49` — current `ExecuteLoopWorkerSpec` exists with the fields the bead lists (and is missing MaxCostUSD/RequestTimeout/MinPower/MaxPower/FromRev as the bead claims). 2. ADR-022 step 7 (ddx-eccc6efb) is OPEN with acceptance: \"Legacy ExecuteLoopWorkerSpec serialization deletes when this lands.\" Confirmed via `ddx bead show ddx-eccc6efb`. 3. ADR-022 step 4 (ddx-85165814) already CLOSED → `--local` is a deprecated no-op (`cli/cmd/agent_cmd.go:1576-1577`). The cobra path no longer branches to server-submit; `executeLoopWithServer` (`cli/cmd/agent_cmd.go:1986`) is unreferenced (verified: only the function definition matches; no call sites). 4. Therefore the bead's USER-OBSERVED SYMPTOM (\"ddx work --harness claude works under --local but fails when going through the server\") cannot be reproduced via the CLI today — `--local` is a no-op and the server-submit branch from cobra is dead code. The remaining server-spawn path is via `POST /api/agent/workers/execute-loop` (`cli/internal/server/server.go:2390`) and the GraphQL `workerDispatchAdapter` (`cli/internal/server/graphql_adapters.go:127`), both of which ADR-022 step 7 wants to delete in favor of subprocess exec. 5. Recent ddx-eccc6efb attempt (20260503T065047-61e4ed20, result.json) was DECLINED for size and decomposed into proposed children 7a/7b/7c — those children may not yet be filed in the main repo per its own bookkeeping note, but the intent stands. ## Size-check verdict (independent of the conflict) The bead also fails the size check on its own: - 17 ACs spanning unrelated subsystems (cobra flag binding, client-side validation, HTTP handler, GraphQL adapter, workers.go consumer, on-disk persistence, backward-compat for legacy fields, forward-compat with SpecVersion, custom Duration codec, reflection-driven test, manual end-to-end run). - Touches at minimum: new package `cli/internal/agent/executeloop/` (struct + Validate + ApplyDefaults + custom Duration + tests), `cli/cmd/agent_cmd.go` (cobra rebind + dispatch split), `cli/internal/server/server.go` (handler rewrite + handler tests), `cli/internal/server/graphql_adapters.go` + `graphql_adapters_default_profile_test.go` + `workers_config_test.go` (parallel-construction deletion), `cli/internal/server/workers.go` (consumer rewrite), and a literal sweep across at least 8 test files (`workers_test.go`, `workers_e2e_test.go`, `workers_prune_test.go`, `workers_stop_test.go`, `workers_stop_propagation_test.go`, `workers_watchdog_test.go`, plus the two GraphQL test files) that all construct `ExecuteLoopWorkerSpec{}` literals. - File-size signal: workers.go 1959 LOC, server.go 5248 LOC, agent_cmd.go 2245 LOC, workers_test.go 1107 LOC. Estimated change footprint comfortably \u003e500 LOC across \u003e5 files in unrelated packages. - Per execution-bead protocol size check: \"More than ~6 ACs spanning unrelated subsystems\" + \"More than ~500 lines across more than ~5 files in unrelated packages\" → DECOMPOSE. ## Why I am not decomposing in this attempt The protocol's decomposition path commits the project to a particular shape (in this case: unification rather than deletion). Filing 4–6 child beads under ddx-29058e2a would advance an architectural direction that is in unresolved conflict with ddx-eccc6efb. A clean decomposition only makes sense AFTER the parent-vs-step-7 design choice is made by a human: Option A — Drop ddx-29058e2a in favor of ADR-022 step 7. The parallel-struct smell goes away by deletion, not unification. The 5 currently-dropped flags can be patched as a narrow fix on the existing struct in the meantime if the operator workflow needs them, with a note that step 7 will erase the struct. Option B — Keep ddx-29058e2a and amend / close ddx-eccc6efb. The unified spec becomes the canonical wire format; ADR-022 step 7 reduces in scope (or is closed as superseded) because the server keeps marshalling a spec, just a unified one. Option C — Sequence: land the narrow 5-flag fix now (small bead), then land ADR-022 step 7 (which deletes the struct and obsoletes the parallel-struct smell), then close ddx-29058e2a as superseded. Each option has different downstream implications (ddx-fb290074 audit scope, REACH-PROTO entry roots, where Validate/ApplyDefaults live, whether SpecVersion is needed at all). I do not have authority to pick. ## Recommended next actions for the orchestrator / triage 1. Decide which of A / B / C above is the intended path. This is a 5-minute design call, not a research task. 2. If A: close ddx-29058e2a as superseded; optionally file a small fix bead \"add MaxCostUSD/RequestTimeout/MinPower/MaxPower/FromRev to ExecuteLoopWorkerSpec + propagate through handler\" (~50 LOC, will be deleted by step 7 anyway). 3. If B: amend ddx-eccc6efb's scope (or close it), then decompose ddx-29058e2a into ~5 children: (i) new spec package + Validate/ApplyDefaults + Duration codec + reflection test, (ii) cobra rebind + parseExecuteLoopSpec + dispatch split, (iii) HTTP handler decode-into-spec rewrite + delete handler-local request struct, (iv) GraphQL adapter unification, (v) workers.go consumer migration + workers_*_test literal sweep + spec.json on-disk format flip + SpecVersion gate. 4. If C: file the narrow 5-flag fix as its own bead now; pause ddx-29058e2a until after step 7 lands, at which point most of its ACs disappear and what's left (if anything) can be re-scoped. ## What this attempt did NOT do (and why) - No code changes. The conflict is upstream of any implementation choice. - No child beads filed. Filing children would commit to the unification path; see \"Why I am not decomposing\" above. - No partial fix to the 5 dropped flags. The bead explicitly absorbs that narrow fix; doing it here without addressing the unification framing would muddy the bead's status.",
+          "created_at": "2026-05-03T11:09:21.004532662Z",
+          "kind": "no_changes_needs_investigation",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes_needs_investigation"
+        },
+        {
+          "actor": "erik",
+          "body": "no_changes\nrationale: status: needs_investigation\nreason: Architectural conflict with open bead ddx-eccc6efb (ADR-022 step 7), plus the bead materially exceeds the size-check thresholds. Human triage required to resolve the design conflict before any unification work commits.\n\n## Summary\n\nThe bead asks to UNIFY the parallel ExecuteLoopWorkerSpec / cobra-flag / HTTP-request structs into one canonical `ExecuteLoopSpec` struct used at every layer. But the open ADR-022 step 7 epic (ddx-eccc6efb, status=open) takes the opposite direction: DELETE `ExecuteLoopWorkerSpec` entirely and replace the in-process server-spawn path with `exec ddx work` (subprocess), at which point there are no parallel structs because the server stops marshalling a spec at all — it just builds an argv vector.\n\nThese are mutually-exclusive architectural directions for the same code. Landing this bead's unification would either:\n  (a) be partially undone when ADR-022 step 7 deletes the unified struct, or\n  (b) lock in the unified-struct design and require ADR-022 step 7 to be re-scoped or abandoned.\n\nThe bead's \"SEQUENCING WITH IN-FLIGHT BEADS\" section names dc157075, 9d55601f, and a946c744, but is silent on ddx-eccc6efb / ADR-022 step 7. That gap is the load-bearing decision this bead does not make.\n\n## Evidence\n\n1. `cli/internal/server/workers.go:25-49` — current `ExecuteLoopWorkerSpec` exists with the fields the bead lists (and is missing MaxCostUSD/RequestTimeout/MinPower/MaxPower/FromRev as the bead claims).\n2. ADR-022 step 7 (ddx-eccc6efb) is OPEN with acceptance: \"Legacy ExecuteLoopWorkerSpec serialization deletes when this lands.\" Confirmed via `ddx bead show ddx-eccc6efb`.\n3. ADR-022 step 4 (ddx-85165814) already CLOSED → `--local` is a deprecated no-op (`cli/cmd/agent_cmd.go:1576-1577`). The cobra path no longer branches to server-submit; `executeLoopWithServer` (`cli/cmd/agent_cmd.go:1986`) is unreferenced (verified: only the function definition matches; no call sites).\n4. Therefore the bead's USER-OBSERVED SYMPTOM (\"ddx work --harness claude works under --local but fails when going through the server\") cannot be reproduced via the CLI today — `--local` is a no-op and the server-submit branch from cobra is dead code. The remaining server-spawn path is via `POST /api/agent/workers/execute-loop` (`cli/internal/server/server.go:2390`) and the GraphQL `workerDispatchAdapter` (`cli/internal/server/graphql_adapters.go:127`), both of which ADR-022 step 7 wants to delete in favor of subprocess exec.\n5. Recent ddx-eccc6efb attempt (20260503T065047-61e4ed20, result.json) was DECLINED for size and decomposed into proposed children 7a/7b/7c — those children may not yet be filed in the main repo per its own bookkeeping note, but the intent stands.\n\n## Size-check verdict (independent of the conflict)\n\nThe bead also fails the size check on its own:\n\n- 17 ACs spanning unrelated subsystems (cobra flag binding, client-side validation, HTTP handler, GraphQL adapter, workers.go consumer, on-disk persistence, backward-compat for legacy fields, forward-compat with SpecVersion, custom Duration codec, reflection-driven test, manual end-to-end run).\n- Touches at minimum: new package `cli/internal/agent/executeloop/` (struct + Validate + ApplyDefaults + custom Duration + tests), `cli/cmd/agent_cmd.go` (cobra rebind + dispatch split), `cli/internal/server/server.go` (handler rewrite + handler tests), `cli/internal/server/graphql_adapters.go` + `graphql_adapters_default_profile_test.go` + `workers_config_test.go` (parallel-construction deletion), `cli/internal/server/workers.go` (consumer rewrite), and a literal sweep across at least 8 test files (`workers_test.go`, `workers_e2e_test.go`, `workers_prune_test.go`, `workers_stop_test.go`, `workers_stop_propagation_test.go`, `workers_watchdog_test.go`, plus the two GraphQL test files) that all construct `ExecuteLoopWorkerSpec{}` literals.\n- File-size signal: workers.go 1959 LOC, server.go 5248 LOC, agent_cmd.go 2245 LOC, workers_test.go 1107 LOC. Estimated change footprint comfortably \u003e500 LOC across \u003e5 files in unrelated packages.\n- Per execution-bead protocol size check: \"More than ~6 ACs spanning unrelated subsystems\" + \"More than ~500 lines across more than ~5 files in unrelated packages\" → DECOMPOSE.\n\n## Why I am not decomposing in this attempt\n\nThe protocol's decomposition path commits the project to a particular shape (in this case: unification rather than deletion). Filing 4–6 child beads under ddx-29058e2a would advance an architectural direction that is in unresolved conflict with ddx-eccc6efb. A clean decomposition only makes sense AFTER the parent-vs-step-7 design choice is made by a human:\n\n  Option A — Drop ddx-29058e2a in favor of ADR-022 step 7. The parallel-struct smell goes away by deletion, not unification. The 5 currently-dropped flags can be patched as a narrow fix on the existing struct in the meantime if the operator workflow needs them, with a note that step 7 will erase the struct.\n  Option B — Keep ddx-29058e2a and amend / close ddx-eccc6efb. The unified spec becomes the canonical wire format; ADR-022 step 7 reduces in scope (or is closed as superseded) because the server keeps marshalling a spec, just a unified one.\n  Option C — Sequence: land the narrow 5-flag fix now (small bead), then land ADR-022 step 7 (which deletes the struct and obsoletes the parallel-struct smell), then close ddx-29058e2a as superseded.\n\nEach option has different downstream implications (ddx-fb290074 audit scope, REACH-PROTO entry roots, where Validate/ApplyDefaults live, whether SpecVersion is needed at all). I do not have authority to pick.\n\n## Recommended next actions for the orchestrator / triage\n\n1. Decide which of A / B / C above is the intended path. This is a 5-minute design call, not a research task.\n2. If A: close ddx-29058e2a as superseded; optionally file a small fix bead \"add MaxCostUSD/RequestTimeout/MinPower/MaxPower/FromRev to ExecuteLoopWorkerSpec + propagate through handler\" (~50 LOC, will be deleted by step 7 anyway).\n3. If B: amend ddx-eccc6efb's scope (or close it), then decompose ddx-29058e2a into ~5 children: (i) new spec package + Validate/ApplyDefaults + Duration codec + reflection test, (ii) cobra rebind + parseExecuteLoopSpec + dispatch split, (iii) HTTP handler decode-into-spec rewrite + delete handler-local request struct, (iv) GraphQL adapter unification, (v) workers.go consumer migration + workers_*_test literal sweep + spec.json on-disk format flip + SpecVersion gate.\n4. If C: file the narrow 5-flag fix as its own bead now; pause ddx-29058e2a until after step 7 lands, at which point most of its ACs disappear and what's left (if anything) can be re-scoped.\n\n## What this attempt did NOT do (and why)\n\n- No code changes. The conflict is upstream of any implementation choice.\n- No child beads filed. Filing children would commit to the unification path; see \"Why I am not decomposing\" above.\n- No partial fix to the 5 dropped flags. The bead explicitly absorbs that narrow fix; doing it here without addressing the unification framing would muddy the bead's status.\nresult_rev=961c8049ea9842f92d5d526429575ef605bb59c4\nbase_rev=961c8049ea9842f92d5d526429575ef605bb59c4\nretry_after=2026-05-03T17:09:21Z",
+          "created_at": "2026-05-03T11:09:21.079692906Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:09:25.929932109Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:11:52.974801919Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:17:11.968537059Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:22:30.882495045Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:27:49.832798052Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:33:08.855509728Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:38:27.708646022Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:43:47.150250659Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:49:06.15318017Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:54:25.198637161Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T17:59:44.395142844Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:05:04.17803028Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:10:23.746833922Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:15:43.517931038Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:21:03.446655075Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:26:23.150811735Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:31:42.994797676Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:37:02.618663585Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:42:22.518367464Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:47:42.131605633Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:53:02.071640074Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T18:58:21.900531146Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:03:41.886490094Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:09:01.582707392Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:14:21.542038158Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:19:41.181105285Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:25:01.162681337Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"detail\":\"routing preflight rejected (harness=codex model=): no viable routing candidate for pins harness=codex: 1 candidates rejected\",\"harness\":\"codex\",\"model\":\"\",\"reason\":\"preflight_rejected\"}",
+          "created_at": "2026-05-03T19:30:21.189404199Z",
+          "kind": "disruption_detected",
+          "source": "ddx agent execute-loop",
+          "summary": "preflight_rejected"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-04T02:17:59.050849919Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-05-03T17:09:21Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T021759-c712d0e8",
+    "prompt": ".ddx/executions/20260504T021759-c712d0e8/prompt.md",
+    "manifest": ".ddx/executions/20260504T021759-c712d0e8/manifest.json",
+    "result": ".ddx/executions/20260504T021759-c712d0e8/result.json",
+    "checks": ".ddx/executions/20260504T021759-c712d0e8/checks.json",
+    "usage": ".ddx/executions/20260504T021759-c712d0e8/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-29058e2a-20260504T021759-c712d0e8"
+  },
+  "prompt_sha": "cc8b621970273f0a4d32dfda6a6809665e5923780d34b8064ab664c3e60dbbed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T021759-c712d0e8/result.json b/.ddx/executions/20260504T021759-c712d0e8/result.json
new file mode 100644
index 00000000..d131848e
--- /dev/null
+++ b/.ddx/executions/20260504T021759-c712d0e8/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-29058e2a",
+  "attempt_id": "20260504T021759-c712d0e8",
+  "base_rev": "b21843270d688978cf5d521d4dcba4b4d81e7adf",
+  "result_rev": "4f25f6bf5a9ba07cf517b0eae2564955b0a48b3a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-f56961ad",
+  "duration_ms": 277835,
+  "tokens": 2311988,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T021759-c712d0e8",
+  "prompt_file": ".ddx/executions/20260504T021759-c712d0e8/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T021759-c712d0e8/manifest.json",
+  "result_file": ".ddx/executions/20260504T021759-c712d0e8/result.json",
+  "usage_file": ".ddx/executions/20260504T021759-c712d0e8/usage.json",
+  "started_at": "2026-05-04T02:18:00.789392433Z",
+  "finished_at": "2026-05-04T02:22:38.624717954Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
