<bead-review>
  <bead id="ddx-29058e2a" iter=1>
    <title>agent: unify ExecuteLoopSpec as single source of truth across cobra/HTTP/server/worker layers</title>
    <description>
PROBLEM
ExecuteLoopSpec fields are declared in parallel structs across three independent code sites. When a flag is added to cobra, the developer must also update the REST handler request struct in server.go and the GraphQL adapter request struct in graphql_adapters.go; missing one silently drops the field at runtime. This is the root cause of the five flags (opaque_passthrough, max_cost, request_timeout, min_power, max_power) being dropped on the server-managed-worker path, causing "ddx work --harness claude" to fail harness validation when routed through the server.

ROOT CAUSE
Each execute-loop flag traverses six independent declaration points with no compile-time linkage:

1. cobra flag extract + local var: cli/cmd/agent_cmd.go:1582-1605 (fromRev, harness, model, profile, provider, modelRef, effort, once, pollInterval, noReview, reviewHarness, reviewModel, maxCostUSD, requestTimeout, minPower, maxPower extracted as naked locals)

2. positional-param pass-down: cli/cmd/agent_cmd.go:1744-1773 — these locals are closed over by singleTierAttempt and assembled into config.CLIOverrides by hand; no single struct groups them

3. ExecuteLoopWorkerSpec definition (worker-side canonical): cli/internal/server/workers.go:25-49 — typed, but hand-maintained parallel to cobra; missing MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev

4. handleStartExecuteLoopWorker request struct (REST handler): cli/internal/server/server.go:2395-2409 — SEPARATE anonymous struct; the five dropped flags and OpaquePassthrough are absent here, causing them to be silently lost for server-spawned workers

5. StartExecuteLoop literal: cli/internal/server/server.go:2441-2455 + cli/internal/server/workers.go:345-361 — hand-assembled from the request struct fields; any field absent in step 4 cannot appear here

6. GraphQL adapter request struct + StartExecuteLoop literal: cli/internal/server/graphql_adapters.go:63-76 + 127-141 — a THIRD parallel declaration (OpaquePassthrough absent here too); fixes to the REST path leave GraphQL re-broken

The consumer (cli/internal/server/workers.go:658-926 runWorker) reads MaxCostUSD from escalation.DefaultMaxCostUSD hardcoded (line ~753) instead of spec.MaxCostUSD because the field was never plumbed through. OpaquePassthrough has TWO validation skip gates that must both fire (workers.go:357-361 and the ValidateForExecuteLoopViaService branch in runWorker).

PROPOSED FIX
- cli/internal/agent/spec.go (new file): define ExecuteLoopSpec with ALL fields currently spread across the six layers. Fields: ProjectRoot, Harness, Model, Profile, Provider, ModelRef, Effort, LabelFilter, Once, PollInterval, NoReview, ReviewHarness, ReviewModel, OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev, SpecVersion int. JSON tags drive both REST wire format and disk persistence.
- cli/internal/agent/spec.go: add Spec.Validate() and Spec.ApplyDefaults() methods. Add parseExecuteLoopSpec(cmd *cobra.Command) (ExecuteLoopSpec, DispatchOptions, error) — the single cobra→spec conversion.
- cli/cmd/agent_cmd.go:1582-1605: replace naked-local extraction with parseExecuteLoopSpec call. Remove positional-param closure pattern; pass spec by value.
- cli/internal/server/workers.go:25-49: replace ExecuteLoopWorkerSpec with (or alias to) ExecuteLoopSpec from the new package. Delete the parallel struct.
- cli/internal/server/server.go:2395-2409: delete the anonymous request struct in handleStartExecuteLoopWorker; decode directly into ExecuteLoopSpec. Call ApplyDefaults() then Validate() before StartExecuteLoop.
- cli/internal/server/graphql_adapters.go:63-76 + 127-141: delete the parallel anonymous struct and parallel StartExecuteLoop literal; route through the same server-side ApplyDefaults + Validate + StartExecuteLoop as the REST handler.
- cli/internal/server/workers.go:~753: replace hardcoded escalation.DefaultMaxCostUSD with spec.MaxCostUSD (with DefaultMaxCostUSD as fallback when zero).
- cli/internal/agent/spec_test.go (new file): TestExecuteLoopSpec_RoundTripsAllFields_Reflection uses reflect over exported fields; sets non-zero value per field; marshals; unmarshals; asserts round-trip. New field = automatic coverage.

NON-SCOPE
- ddx-eccc6efb (ADR-022 step 7) owns replacing runWorker with exec ddx work; this bead does NOT change the subprocess vs in-process dispatch decision.
- Other commands (ddx agent run, ddx try, MCP agent dispatch) — same smell may exist; covered by sister bead ddx-fb290074.
- Renaming flags or reshaping CLI surface.
- Reshaping LabelFilter from string to []string — owned by ddx-9d55601f; keep LabelFilter as string.

SEQUENCING
ddx-29058e2a (this bead) vs ddx-eccc6efb (ADR-022 step 7 — server-spawn migration to exec ddx work):
LANDS INDEPENDENTLY and BEFORE ddx-eccc6efb. ADR-022 §"Sequencing relative to in-flight work" (line 553) is explicit: "[ddx-29058e2a] remains valuable as a worker-side struct unification (cobra → in-process flow) and should land independently." When ddx-eccc6efb lands later, it will delete ExecuteLoopWorkerSpec from workers.go as part of the exec migration — but by then, the unified ExecuteLoopSpec already lives in cli/internal/agent/spec.go, which is the correct durable home for it regardless of transport. ddx-eccc6efb does NOT block or supersede this bead; this bead does NOT block ddx-eccc6efb. Coordination note: if ddx-eccc6efb lands first, the scope of this bead narrows (the server.go and graphql_adapters.go layers are already deleted), so cherry-pick only the cobra + workers.go layers.
    </description>
    <acceptance>
1. ExecuteLoopSpec struct defined in cli/internal/agent/spec.go (or cli/internal/agent/executeloop/spec.go). Exported fields include AT MINIMUM: ProjectRoot, Harness, Model, Profile, Provider, ModelRef, Effort, LabelFilter, Once, PollInterval, NoReview, ReviewHarness, ReviewModel, OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev, SpecVersion. Audit: any field present in cobra extraction (agent_cmd.go:1582-1605) or workers.go:25-49 NOT in this list is a bug.

2. parseExecuteLoopSpec(cmd *cobra.Command) (ExecuteLoopSpec, DispatchOptions, error) exists in the new package. Called only from cobra RunE in agent_cmd.go. No parallel cobra-extraction block remains. DispatchOptions holds control-plane flags (--local, --json) and is NOT persisted to spec.json.

3. Spec.Validate() and Spec.ApplyDefaults() methods exist. Lifecycle enforced: (a) client calls Validate() before HTTP submit or inline dispatch; (b) server handleStartExecuteLoopWorker calls ApplyDefaults() then Validate() after decode, before StartExecuteLoop; (c) spec.json on disk is fully-resolved (ApplyDefaults already ran at write time).

4. handleStartExecuteLoopWorker (cli/internal/server/server.go:2390) decodes directly into ExecuteLoopSpec; the anonymous request struct (currently lines 2395-2409) is deleted. OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev all survive the REST round-trip.

5. graphql_adapters.go anonymous request struct (lines 63-76) and its parallel StartExecuteLoop literal (lines 127-141) are deleted. GraphQL dispatch goes through the same server-side ApplyDefaults + Validate + StartExecuteLoop as the REST handler.

6. ExecuteLoopWorkerSpec in cli/internal/server/workers.go:25-49 is replaced by or aliased to ExecuteLoopSpec. spec.json files written by StartExecuteLoop use the unified struct.

7. runWorker (cli/internal/server/workers.go:658) consumes spec.MaxCostUSD as the cost-cap ceiling (with escalation.DefaultMaxCostUSD fallback when zero). No hardcoded DefaultMaxCostUSD reference outside the fallback case.

8. OpaquePassthrough: server-submitted spec with OpaquePassthrough=true skips BOTH validation gates — workers.go:357-361 (StartExecuteLoop pre-flight) AND the ValidateForExecuteLoopViaService branch inside runWorker. Test: TestStartExecuteLoop_OpaquePassthrough_SkipsValidation asserts that StartExecuteLoop with OpaquePassthrough=true does not call ValidateForExecuteLoopViaService and returns a WorkerRecord without error even when the harness name is unresolvable.

9. Reflection round-trip test: TestExecuteLoopSpec_RoundTripsAllFields_Reflection in cli/internal/agent/spec_test.go (or executeloop sub-package). Uses reflect to iterate exported fields of ExecuteLoopSpec; for each field sets a non-zero sentinel value; marshals to JSON; unmarshals; asserts equality. Test fails if any exported field has a zero-value sentinel after unmarshal (catches omitempty bugs that silently drop non-default-false fields). A future added field automatically falls under coverage.

10. Backward compatibility: existing spec.json files in .ddx/workers/ load without error. Specifically: files containing legacy fields (e.g., min_tier, max_tier from older specs) are tolerated and ignored by json.Unmarshal default behavior. Do NOT enable json.Decoder.DisallowUnknownFields for disk reads.

11. Forward compatibility: SpecVersion int field present. Server returns HTTP 400 with a capability description when incoming SpecVersion &gt; server-supported version. SpecVersion==0 (unset) is treated as the current version. Test: TestHandleStartExecuteLoopWorker_HighSpecVersion_Returns400.

12. Regression: --local path (runAgentExecuteLoopImpl inline path) is unchanged in behavior. Existing tests that cover the inline path pass without modification.

13. cd cli &amp;&amp; go test ./cmd/... ./internal/agent/... ./internal/server/... green.

14. lefthook run pre-commit passes.
    </acceptance>
    <notes>
decomposed into ddx-1a9cc01f (canonical ExecuteLoopSpec package), ddx-76cf71f4 (cobra parse/DispatchOptions), ddx-da9e9491 (REST handler decode/validation), ddx-ce1d6309 (GraphQL dispatch unification), ddx-89a9c305 (worker spec/persistence/runtime migration), and ddx-16722d4e (REACH-PROTO entry-root follow-through).

REVIEW:BLOCK

Diff contains only execution-evidence JSON files (manifest.json, result.json). No source code changes exist. None of the 17 acceptance criteria — unified ExecuteLoopSpec struct, DispatchOptions, Validate/ApplyDefaults, custom Duration codec, cobra rebind, REST/GraphQL handler unification, workers.go consumer migration, reflection round-trip test, SpecVersion gate, OpaquePassthrough fix — are implemented here. The bead's notes claim decomposition into 6 children, but no child beads are visible in the changed files either; only evidence artifacts were committed.
    </notes>
    <labels>phase:2, area:agent, area:server, kind:refactor, prevention, triage:needs-investigation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T213511-63e1adf9/manifest.json</file>
    <file>.ddx/executions/20260504T213511-63e1adf9/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f7aff6d7214d1577ea0809861819ab48dc2a9100">
diff --git a/.ddx/executions/20260504T213511-63e1adf9/manifest.json b/.ddx/executions/20260504T213511-63e1adf9/manifest.json
new file mode 100644
index 00000000..35d23008
--- /dev/null
+++ b/.ddx/executions/20260504T213511-63e1adf9/manifest.json
@@ -0,0 +1,886 @@
+{
+  "attempt_id": "20260504T213511-63e1adf9",
+  "bead_id": "ddx-29058e2a",
+  "base_rev": "cf50aaf92d254303ebf0ccdec67865faee8a8eea",
+  "created_at": "2026-05-04T21:35:13.990611291Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-29058e2a",
+    "title": "agent: unify ExecuteLoopSpec as single source of truth across cobra/HTTP/server/worker layers",
+    "description": "PROBLEM\nExecuteLoopSpec fields are declared in parallel structs across three independent code sites. When a flag is added to cobra, the developer must also update the REST handler request struct in server.go and the GraphQL adapter request struct in graphql_adapters.go; missing one silently drops the field at runtime. This is the root cause of the five flags (opaque_passthrough, max_cost, request_timeout, min_power, max_power) being dropped on the server-managed-worker path, causing \"ddx work --harness claude\" to fail harness validation when routed through the server.\n\nROOT CAUSE\nEach execute-loop flag traverses six independent declaration points with no compile-time linkage:\n\n1. cobra flag extract + local var: cli/cmd/agent_cmd.go:1582-1605 (fromRev, harness, model, profile, provider, modelRef, effort, once, pollInterval, noReview, reviewHarness, reviewModel, maxCostUSD, requestTimeout, minPower, maxPower extracted as naked locals)\n\n2. positional-param pass-down: cli/cmd/agent_cmd.go:1744-1773 — these locals are closed over by singleTierAttempt and assembled into config.CLIOverrides by hand; no single struct groups them\n\n3. ExecuteLoopWorkerSpec definition (worker-side canonical): cli/internal/server/workers.go:25-49 — typed, but hand-maintained parallel to cobra; missing MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev\n\n4. handleStartExecuteLoopWorker request struct (REST handler): cli/internal/server/server.go:2395-2409 — SEPARATE anonymous struct; the five dropped flags and OpaquePassthrough are absent here, causing them to be silently lost for server-spawned workers\n\n5. StartExecuteLoop literal: cli/internal/server/server.go:2441-2455 + cli/internal/server/workers.go:345-361 — hand-assembled from the request struct fields; any field absent in step 4 cannot appear here\n\n6. GraphQL adapter request struct + StartExecuteLoop literal: cli/internal/server/graphql_adapters.go:63-76 + 127-141 — a THIRD parallel declaration (OpaquePassthrough absent here too); fixes to the REST path leave GraphQL re-broken\n\nThe consumer (cli/internal/server/workers.go:658-926 runWorker) reads MaxCostUSD from escalation.DefaultMaxCostUSD hardcoded (line ~753) instead of spec.MaxCostUSD because the field was never plumbed through. OpaquePassthrough has TWO validation skip gates that must both fire (workers.go:357-361 and the ValidateForExecuteLoopViaService branch in runWorker).\n\nPROPOSED FIX\n- cli/internal/agent/spec.go (new file): define ExecuteLoopSpec with ALL fields currently spread across the six layers. Fields: ProjectRoot, Harness, Model, Profile, Provider, ModelRef, Effort, LabelFilter, Once, PollInterval, NoReview, ReviewHarness, ReviewModel, OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev, SpecVersion int. JSON tags drive both REST wire format and disk persistence.\n- cli/internal/agent/spec.go: add Spec.Validate() and Spec.ApplyDefaults() methods. Add parseExecuteLoopSpec(cmd *cobra.Command) (ExecuteLoopSpec, DispatchOptions, error) — the single cobra→spec conversion.\n- cli/cmd/agent_cmd.go:1582-1605: replace naked-local extraction with parseExecuteLoopSpec call. Remove positional-param closure pattern; pass spec by value.\n- cli/internal/server/workers.go:25-49: replace ExecuteLoopWorkerSpec with (or alias to) ExecuteLoopSpec from the new package. Delete the parallel struct.\n- cli/internal/server/server.go:2395-2409: delete the anonymous request struct in handleStartExecuteLoopWorker; decode directly into ExecuteLoopSpec. Call ApplyDefaults() then Validate() before StartExecuteLoop.\n- cli/internal/server/graphql_adapters.go:63-76 + 127-141: delete the parallel anonymous struct and parallel StartExecuteLoop literal; route through the same server-side ApplyDefaults + Validate + StartExecuteLoop as the REST handler.\n- cli/internal/server/workers.go:~753: replace hardcoded escalation.DefaultMaxCostUSD with spec.MaxCostUSD (with DefaultMaxCostUSD as fallback when zero).\n- cli/internal/agent/spec_test.go (new file): TestExecuteLoopSpec_RoundTripsAllFields_Reflection uses reflect over exported fields; sets non-zero value per field; marshals; unmarshals; asserts round-trip. New field = automatic coverage.\n\nNON-SCOPE\n- ddx-eccc6efb (ADR-022 step 7) owns replacing runWorker with exec ddx work; this bead does NOT change the subprocess vs in-process dispatch decision.\n- Other commands (ddx agent run, ddx try, MCP agent dispatch) — same smell may exist; covered by sister bead ddx-fb290074.\n- Renaming flags or reshaping CLI surface.\n- Reshaping LabelFilter from string to []string — owned by ddx-9d55601f; keep LabelFilter as string.\n\nSEQUENCING\nddx-29058e2a (this bead) vs ddx-eccc6efb (ADR-022 step 7 — server-spawn migration to exec ddx work):\nLANDS INDEPENDENTLY and BEFORE ddx-eccc6efb. ADR-022 §\"Sequencing relative to in-flight work\" (line 553) is explicit: \"[ddx-29058e2a] remains valuable as a worker-side struct unification (cobra → in-process flow) and should land independently.\" When ddx-eccc6efb lands later, it will delete ExecuteLoopWorkerSpec from workers.go as part of the exec migration — but by then, the unified ExecuteLoopSpec already lives in cli/internal/agent/spec.go, which is the correct durable home for it regardless of transport. ddx-eccc6efb does NOT block or supersede this bead; this bead does NOT block ddx-eccc6efb. Coordination note: if ddx-eccc6efb lands first, the scope of this bead narrows (the server.go and graphql_adapters.go layers are already deleted), so cherry-pick only the cobra + workers.go layers.",
+    "acceptance": "1. ExecuteLoopSpec struct defined in cli/internal/agent/spec.go (or cli/internal/agent/executeloop/spec.go). Exported fields include AT MINIMUM: ProjectRoot, Harness, Model, Profile, Provider, ModelRef, Effort, LabelFilter, Once, PollInterval, NoReview, ReviewHarness, ReviewModel, OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev, SpecVersion. Audit: any field present in cobra extraction (agent_cmd.go:1582-1605) or workers.go:25-49 NOT in this list is a bug.\n\n2. parseExecuteLoopSpec(cmd *cobra.Command) (ExecuteLoopSpec, DispatchOptions, error) exists in the new package. Called only from cobra RunE in agent_cmd.go. No parallel cobra-extraction block remains. DispatchOptions holds control-plane flags (--local, --json) and is NOT persisted to spec.json.\n\n3. Spec.Validate() and Spec.ApplyDefaults() methods exist. Lifecycle enforced: (a) client calls Validate() before HTTP submit or inline dispatch; (b) server handleStartExecuteLoopWorker calls ApplyDefaults() then Validate() after decode, before StartExecuteLoop; (c) spec.json on disk is fully-resolved (ApplyDefaults already ran at write time).\n\n4. handleStartExecuteLoopWorker (cli/internal/server/server.go:2390) decodes directly into ExecuteLoopSpec; the anonymous request struct (currently lines 2395-2409) is deleted. OpaquePassthrough, MaxCostUSD, RequestTimeout, MinPower, MaxPower, FromRev all survive the REST round-trip.\n\n5. graphql_adapters.go anonymous request struct (lines 63-76) and its parallel StartExecuteLoop literal (lines 127-141) are deleted. GraphQL dispatch goes through the same server-side ApplyDefaults + Validate + StartExecuteLoop as the REST handler.\n\n6. ExecuteLoopWorkerSpec in cli/internal/server/workers.go:25-49 is replaced by or aliased to ExecuteLoopSpec. spec.json files written by StartExecuteLoop use the unified struct.\n\n7. runWorker (cli/internal/server/workers.go:658) consumes spec.MaxCostUSD as the cost-cap ceiling (with escalation.DefaultMaxCostUSD fallback when zero). No hardcoded DefaultMaxCostUSD reference outside the fallback case.\n\n8. OpaquePassthrough: server-submitted spec with OpaquePassthrough=true skips BOTH validation gates — workers.go:357-361 (StartExecuteLoop pre-flight) AND the ValidateForExecuteLoopViaService branch inside runWorker. Test: TestStartExecuteLoop_OpaquePassthrough_SkipsValidation asserts that StartExecuteLoop with OpaquePassthrough=true does not call ValidateForExecuteLoopViaService and returns a WorkerRecord without error even when the harness name is unresolvable.\n\n9. Reflection round-trip test: TestExecuteLoopSpec_RoundTripsAllFields_Reflection in cli/internal/agent/spec_test.go (or executeloop sub-package). Uses reflect to iterate exported fields of ExecuteLoopSpec; for each field sets a non-zero sentinel value; marshals to JSON; unmarshals; asserts equality. Test fails if any exported field has a zero-value sentinel after unmarshal (catches omitempty bugs that silently drop non-default-false fields). A future added field automatically falls under coverage.\n\n10. Backward compatibility: existing spec.json files in .ddx/workers/ load without error. Specifically: files containing legacy fields (e.g., min_tier, max_tier from older specs) are tolerated and ignored by json.Unmarshal default behavior. Do NOT enable json.Decoder.DisallowUnknownFields for disk reads.\n\n11. Forward compatibility: SpecVersion int field present. Server returns HTTP 400 with a capability description when incoming SpecVersion \u003e server-supported version. SpecVersion==0 (unset) is treated as the current version. Test: TestHandleStartExecuteLoopWorker_HighSpecVersion_Returns400.\n\n12. Regression: --local path (runAgentExecuteLoopImpl inline path) is unchanged in behavior. Existing tests that cover the inline path pass without modification.\n\n13. cd cli \u0026\u0026 go test ./cmd/... ./internal/agent/... ./internal/server/... green.\n\n14. lefthook run pre-commit passes.",
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
+      "claimed-at": "2026-05-04T21:35:11Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3748035",
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
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-04T02:22:45.217797402Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex"
+        },
+        {
+          "actor": "erik",
+          "body": "Diff contains only execution-evidence JSON files (manifest.json, result.json). No source code changes exist. None of the 17 acceptance criteria — unified ExecuteLoopSpec struct, DispatchOptions, Validate/ApplyDefaults, custom Duration codec, cobra rebind, REST/GraphQL handler unification, workers.go consumer migration, reflection round-trip test, SpecVersion gate, OpaquePassthrough fix — are implemented here. The bead's notes claim decomposition into 6 children, but no child beads are visible in the changed files either; only evidence artifacts were committed.\nharness=claude\nmodel=opus\ninput_bytes=90350\noutput_bytes=2749\nelapsed_ms=89074",
+          "created_at": "2026-05-04T02:24:14.741599783Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-05-04T02:24:15.001933586Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"action\":\"re_attempt_with_context\",\"mode\":\"review_block\"}",
+          "created_at": "2026-05-04T02:24:15.220582446Z",
+          "kind": "triage-decision",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block: re_attempt_with_context"
+        },
+        {
+          "actor": "erik",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff contains only execution-evidence JSON files (manifest.json, result.json). No source code changes exist. None of the 17 acceptance criteria — unified ExecuteLoopSpec struct, DispatchOptions, Validate/ApplyDefaults, custom Duration codec, cobra rebind, REST/GraphQL handler unification, workers.go consumer migration, reflection round-trip test, SpecVersion gate, OpaquePassthrough fix — are implemented here. The bead's notes claim decomposition into 6 children, but no child beads are visible in the changed files either; only evidence artifacts were committed.\nresult_rev=0e497b4040a88f26c58a99dd272bec5992b9f23e\nbase_rev=b21843270d688978cf5d521d4dcba4b4d81e7adf",
+          "created_at": "2026-05-04T02:24:15.609784235Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-04T21:35:11.812814848Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-05-03T17:09:21Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T213511-63e1adf9",
+    "prompt": ".ddx/executions/20260504T213511-63e1adf9/prompt.md",
+    "manifest": ".ddx/executions/20260504T213511-63e1adf9/manifest.json",
+    "result": ".ddx/executions/20260504T213511-63e1adf9/result.json",
+    "checks": ".ddx/executions/20260504T213511-63e1adf9/checks.json",
+    "usage": ".ddx/executions/20260504T213511-63e1adf9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-29058e2a-20260504T213511-63e1adf9"
+  },
+  "prompt_sha": "ecac392471e7a81d8a7fb3c6c2eeeffc776d6175c4466e7602824402f5e81289"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T213511-63e1adf9/result.json b/.ddx/executions/20260504T213511-63e1adf9/result.json
new file mode 100644
index 00000000..e1c412f0
--- /dev/null
+++ b/.ddx/executions/20260504T213511-63e1adf9/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-29058e2a",
+  "attempt_id": "20260504T213511-63e1adf9",
+  "base_rev": "cf50aaf92d254303ebf0ccdec67865faee8a8eea",
+  "result_rev": "927924f1dd1a2ab353ff9b6963fe7a3e92d3b61b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "session_id": "eb-247f9920",
+  "duration_ms": 50891,
+  "tokens": 447009,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T213511-63e1adf9",
+  "prompt_file": ".ddx/executions/20260504T213511-63e1adf9/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T213511-63e1adf9/manifest.json",
+  "result_file": ".ddx/executions/20260504T213511-63e1adf9/result.json",
+  "usage_file": ".ddx/executions/20260504T213511-63e1adf9/usage.json",
+  "started_at": "2026-05-04T21:35:13.991509041Z",
+  "finished_at": "2026-05-04T21:36:04.882695514Z"
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
