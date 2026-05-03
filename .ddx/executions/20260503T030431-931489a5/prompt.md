<bead-review>
  <bead id="ddx-50da9674" iter=1>
    <title>infra: reusable clean fixture repo for tests + acceptance demos</title>
    <description>
Multiple test/demo workflows need a clean ddx-initialized git repo without polluting this main project. Today each test creates its own t.TempDir() fixture; integration tests do ad-hoc setup; acceptance demos have nowhere to go.

USE CASES (current + planned)
- ADR-022 acceptance demo (Stage 4 of design-phase plan): run 2 workers, restart server mid-execution, verify worker resilience without polluting the ddx repo
- Integration tests that need a real .ddx/ directory + bead store + workers/ + executions/
- bd/br interchange tests
- Cross-project leak tests (need &gt;1 project — already partially used in ddx-4c51d33e LAYER 1 tests)
- Manual operator walkthroughs / demos for new features
- "What does ddx look like in a fresh project?" docs

DESIGN OPTIONS

Option 1: A SCRIPT that builds a fixture repo on demand
- scripts/build-fixture-repo.sh &lt;dest&gt; [--profile minimal|standard|multi-project|federated]
- Creates a tmp git repo, runs ddx init, optionally adds N seed beads, optionally registers projects
- Tests call it via testing.T helper; demos call it from the operator's shell
- Cleanup is operator/test responsibility (rm -rf &lt;dest&gt;)

Option 2: A FIXTURE-REPO BINARY (ddx fixture create)
- ddx fixture create &lt;dest&gt; --profile standard
- Same outputs as Option 1 but discoverable via --help
- Could be a hidden subcommand under ddx (Hidden: true) since it's dev infrastructure
- Uniform with rest of CLI

Option 3: GO TESTING HELPER (testing-only, not exposed to operator)
- testutil.NewFixtureRepo(t, profile) → projectPath
- Auto-cleanup via t.Cleanup()
- Doesn't help acceptance demos but cleanest for unit/integration tests

RECOMMEND: Hybrid. Option 1 (script) for demo + manual use; Option 3 (Go helper) wraps the script for tests so cleanup is automatic. Option 2 deferred unless operators ask for it.

PROFILES TO SUPPORT (initial)
- minimal: empty .ddx/, no beads
- standard: .ddx/ + 5 mixed-priority sample beads + sample personas + sample skills
- multi-project: 2 registered projects, each with their own beads (for cross-project tests)
- federated: hub + spoke setup (for ADR-007 federation tests)

NOT IN SCOPE
- Mocking the agent service (separate concern; tests use stub fizeau already)
- Pre-populating with real LLM execution traces (deterministic test runs use fixtures, not live LLMs)
- Auto-cleanup beyond t.Cleanup() (operators run rm -rf)

DEPS
- Independent; can land anytime.
    </description>
    <acceptance>
1. scripts/build-fixture-repo.sh exists; creates a clean ddx-initialized git repo at &lt;dest&gt;; exits 0 on success.
2. Script supports --profile {minimal,standard,multi-project,federated}; each profile produces the documented seed contents.
3. Go testing helper testutil.NewFixtureRepo(t *testing.T, profile string) (projectPath string) wraps the script; auto-cleanup via t.Cleanup(); usable from any cli/internal/*/test file.
4. At least one existing integration test migrates to use the helper (e.g., one of the cross-project tests in graphql_scoped_route_test.go) — proves the API works in practice.
5. Documentation: short README at scripts/build-fixture-repo.md describing the profiles + usage; one paragraph in CLAUDE.md or testing.md pointing at the helper.
6. cd cli &amp;&amp; go test ./internal/server/... green; lefthook pre-commit passes.
7. Manual verification: bash scripts/build-fixture-repo.sh /tmp/ddx-fixture-test --profile standard &amp;&amp; cd /tmp/ddx-fixture-test &amp;&amp; ddx bead ready # shows the seeded beads.
    </acceptance>
    <labels>phase:2, area:infra, kind:test-infra, reusable</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T025022-29ef49b0/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="36dbc04b113889d5089c94c24b87a1d55c67e188">
diff --git a/.ddx/executions/20260503T025022-29ef49b0/result.json b/.ddx/executions/20260503T025022-29ef49b0/result.json
new file mode 100644
index 00000000..c3e6af4d
--- /dev/null
+++ b/.ddx/executions/20260503T025022-29ef49b0/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-50da9674",
+  "attempt_id": "20260503T025022-29ef49b0",
+  "base_rev": "0a688a9eebf6182a50db2ea417a4efb9986d5a09",
+  "result_rev": "887c862b0d5d96e7a6f11f3edd2b67011582a461",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c32417e0",
+  "duration_ms": 841122,
+  "tokens": 23136,
+  "cost_usd": 2.67096525,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T025022-29ef49b0",
+  "prompt_file": ".ddx/executions/20260503T025022-29ef49b0/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T025022-29ef49b0/manifest.json",
+  "result_file": ".ddx/executions/20260503T025022-29ef49b0/result.json",
+  "usage_file": ".ddx/executions/20260503T025022-29ef49b0/usage.json",
+  "started_at": "2026-05-03T02:50:24.049285632Z",
+  "finished_at": "2026-05-03T03:04:25.171368766Z"
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
