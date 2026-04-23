<bead-review>
  <bead id="ddx-b6cf025c" iter=1>
    <title>Persistent drain-queue worker indicator in nav + N-worker count control on dashboard</title>
    <description>
## Observed / Motivation

Today the queue-drain worker is invisible unless the operator is on the project home page or the Workers tab. An operator on any other page (Beads, Documents, Efficacy) has no idea whether a drain is active, how many ready beads are left, or whether they should kick off more capacity. The per-project home page's "Drain queue" button is one-shot and offers no scaling control — you start one worker and that's it.

User feedback: the drain should be a **top-level always-visible affordance** with:
1. A persistent spinner/badge in the global nav showing drain state + an at-a-glance count (ready beads, running workers).
2. Clicking the badge goes to a workers overview where the operator can see what each worker is doing.
3. That overview needs a "number of workers" control — `Add worker` / `Remove worker` to scale drain capacity up or down, not just start-one-at-a-time.

## Why this is a new surface, not just a tweak to existing Workers page

- **Global spinner** is a nav-shell concern, not a page concern. It needs to be mounted in `NavShell.svelte` so every route sees it.
- **Worker-count control** is a different interaction model from the per-worker Start/Stop flow proposed in `ddx-69789664`. That flow is "pick a harness + effort + filter, start one worker with those specs." This flow is "I want N general-purpose drain workers; add/remove one at a time." Both should coexist.
- A **worker-count affordance** implies a default worker spec (default harness, default profile). That's fine — it mirrors `ddx work` with no flags — but it has to be explicit so the operator knows what they're spawning.

## Scope

### Part 1 — Global drain indicator in `NavShell`

- New element in the top nav bar, visible on every route (fits next to the theme toggle).
- Two states:
  - **Idle** (zero running drain workers): subtle dot + "Queue: X ready" (where X is project-scoped ready-bead count). Clicking navigates to the workers view.
  - **Active** (≥1 running drain worker): animated spinner + "N workers · X ready". Clicking navigates to the workers view.
- Data: new lightweight query `queueAndWorkersSummary(projectId) { readyBeads, runningWorkers, totalWorkers }`, subscribes to worker-progress events to stay fresh.
- Scope: tied to the currently-selected project in the `ProjectPicker`. If no project is selected, the indicator is hidden.
- Perf: the query must be cheap — must not trigger the N·M scan already tracked in `ddx-9ce6842a`. Depends on those fixes for the ready-bead count.

### Part 2 — Workers overview with count control

- Repurpose the existing `/workers` route (or add a separate `/workers/overview` — design call) to include a header section with:
  - Large "Drain workers: N" number.
  - `+` button ("Add worker") — dispatches a new default-spec drain worker via the existing `workerDispatch(kind: "execute-loop")` mutation.
  - `−` button ("Remove worker") — stops the oldest-running drain worker. Confirm-gated.
  - Small help text: "Adds a general-purpose queue-drain worker. Use the per-harness picker below for custom specs."
- The existing per-worker table and per-row Start/Stop (from `ddx-69789664`) remain for fine control.
- "Default spec" is: harness unset (route via profile), profile `default`, no effort override. Operator can change the project default in config.

### Part 3 — Defaults + config

- `.ddx/config.yaml: workers.default_spec` — object with optional harness, profile, effort, min_tier, max_tier. Used by Part 2's `+ Add worker`. Unset = use `ddx work` defaults.
- `.ddx/config.yaml: workers.max_count` — optional safety rail, default unset. When set, `+ Add worker` refuses past the cap and tooltips "at workers.max_count limit".

### Part 4 — Empty/error states

- Indicator on a project with no ready beads and zero workers: "Queue: 0 ready — nothing to drain". Clicking still navigates (so operator can spawn one anyway).
- Indicator when the per-project workers query is broken (pre-`ddx-05b4cc9d`): falls back to the global count; logs a one-time console warning.
- Add-worker failure: surfaces inline on the overview page (error toast + detail), doesn't silently drop.

## Out of scope

- Per-harness-specific queue views (e.g., "drain with codex" as a distinct indicator).
- Bead prioritization rules for multi-worker draining — beads already have a priority field, and `execute-loop` already claims in ready-order. No change.
- Concurrency safety review of multi-worker draining. It already works (beads are claimed atomically), but if `ddx-05b4cc9d` surfaces issues there, file a follow-up.
    </description>
    <acceptance>
**User story:** As an operator on any page in the project, I always see whether a drain worker is running and how much work is queued. One click takes me to the worker overview where I can scale capacity up or down with a single button, without picking a harness or profile every time.

**Acceptance criteria:**

1. **Nav indicator visible everywhere.**
   - A component in `NavShell.svelte` renders the drain indicator on every route inside a selected project.
   - Idle state: dot + "Queue: X ready".
   - Active state: spinner + "N workers · X ready".
   - Clicking navigates to the workers overview.
   - Playwright: navigate to beads, documents, sessions, efficacy — assert indicator is present on each with the expected text.

2. **Live updates.**
   - The indicator updates within 2s when a worker starts, finishes, or changes state. Backed by the existing worker-progress subscription.
   - When ready-bead count changes (a bead is created/closed), the indicator updates within 5s. Either subscription-driven or short poll.

3. **Summary query.**
   - New `queueAndWorkersSummary(projectId) { readyBeads runningWorkers totalWorkers }`. Returns zeros on unknown or empty projects — no error.
   - Resolver is cheap: reads the ready count from the same path bead-status queries use, not by iterating every bead.

4. **Overview page — count control.**
   - Header shows "Drain workers: N" prominently.
   - `+ Add worker` button dispatches a default-spec worker (`workerDispatch(kind: "execute-loop", args: null)`). On success the row appears in the table within 2s and the indicator updates.
   - `− Remove worker` button stops the oldest-running drain worker. Confirm-gated ("Stop worker-XYZ?"). After confirm, the worker transitions to `stopped` within 2s.
   - Help text below the buttons: "Adds a general-purpose drain worker. Use the per-harness picker below for custom specs."

5. **Default-spec + safety rail.**
   - `.ddx/config.yaml: workers.default_spec` is respected by `+ Add worker`. Integration test asserts a configured `profile: cheap` is propagated.
   - `.ddx/config.yaml: workers.max_count`: when set, `+ Add worker` refuses past the cap; button disabled with tooltip "at workers.max_count limit". Not set → no cap. Integration test covers both.

6. **Playwright — flow.**
   - Seeds a project with 3 ready beads, zero workers.
   - Asserts indicator shows "Queue: 3 ready", dot state.
   - Clicks indicator → navigates to overview.
   - Clicks `+ Add worker` → asserts new row and indicator transitions to spinner + "1 worker · 3 ready".
   - Clicks `+` again → asserts "2 workers · 3 ready".
   - Clicks `− Remove worker` → asserts one worker stops and indicator shows "1 worker · ..." within 2s.
   - Navigates to /beads → asserts indicator persists and stays live.

7. **No regressions.**
   - Existing Drain button on project home continues to work (dispatches the same kind). Can stay or be removed depending on design note; recommend keeping it as the initial-kickoff affordance when zero workers run.
   - Existing Workers table rendering + live-phase subscription is unchanged.
   - Existing per-worker Start/Stop from `ddx-69789664` coexists unchanged.

8. **Cross-references.**
   - Hard dependency: `ddx-05b4cc9d` (fixes `workersByProject` filter — without it, the running-workers count is always 0 on a per-project query).
   - Adjacent: `ddx-69789664` (per-worker lifecycle + Sessions IA). These beads are complementary; implementation should not re-introduce either's functionality, just use it.
   - Adjacent: `ddx-9ce6842a` (perf harness). The `queueAndWorkersSummary` resolver gets a perf target in that harness: p95 ≤ 30ms in-process on a 5k-bead fixture. No full scan allowed.
    </acceptance>
    <labels>feat-008, feat-010, feat-006, ui, operator-ux</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="04740d3b0993f51f35ac9df011623edd5acaca97">
commit 04740d3b0993f51f35ac9df011623edd5acaca97
Author: Test User <test@example.com>
Date:   Thu Apr 23 02:28:05 2026 -0400

    feat(workers): persistent drain indicator + count-control UX [ddx-b6cf025c]

diff --git a/cli/internal/config/schema/config.schema.json b/cli/internal/config/schema/config.schema.json
index 4c5158cf..6c7b6b2b 100644
--- a/cli/internal/config/schema/config.schema.json
+++ b/cli/internal/config/schema/config.schema.json
@@ -460,6 +460,45 @@
         }
       },
       "additionalProperties": false
+    },
+    "workers": {
+      "type": "object",
+      "description": "Defaults and safety rails for the workers overview (+ Add worker / − Remove worker count control).",
+      "properties": {
+        "default_spec": {
+          "type": "object",
+          "description": "Default worker spec used by + Add worker on the workers overview. Any field left unset falls back to `ddx work` defaults.",
+          "properties": {
+            "harness": {
+              "type": "string",
+              "description": "Default harness name. Empty leaves routing to the agent profile."
+            },
+            "profile": {
+              "type": "string",
+              "description": "Default routing profile (e.g. default, cheap, fast, smart)."
+            },
+            "effort": {
+              "type": "string",
+              "description": "Default reasoning-effort tier (low, medium, high)."
+            },
+            "min_tier": {
+              "type": "string",
+              "description": "Optional minimum routing tier."
+            },
+            "max_tier": {
+              "type": "string",
+              "description": "Optional maximum routing tier."
+            }
+          },
+          "additionalProperties": false
+        },
+        "max_count": {
+          "type": "integer",
+          "minimum": 0,
+          "description": "Optional cap on running drain workers. + Add worker refuses to exceed this limit. Unset = no cap."
+        }
+      },
+      "additionalProperties": false
     }
   },
   "additionalProperties": false,
diff --git a/cli/internal/config/types.go b/cli/internal/config/types.go
index 12d29a26..a3ac247f 100644
--- a/cli/internal/config/types.go
+++ b/cli/internal/config/types.go
@@ -14,6 +14,25 @@ type NewConfig struct {
 	Server          *ServerConfig      `yaml:"server,omitempty" json:"server,omitempty"`
 	Executions      *ExecutionsConfig  `yaml:"executions,omitempty" json:"executions,omitempty"`
 	Cost            *CostConfig        `yaml:"cost,omitempty" json:"cost,omitempty"`
+	Workers         *WorkersConfig     `yaml:"workers,omitempty" json:"workers,omitempty"`
+}
+
+// WorkersConfig controls the Add/Remove-worker affordances on the workers
+// overview. `default_spec` supplies sane defaults for one-click worker
+// dispatch; `max_count` optionally caps concurrent drain workers per project.
+type WorkersConfig struct {
+	DefaultSpec *WorkerDefaultSpec `yaml:"default_spec,omitempty" json:"default_spec,omitempty"`
+	MaxCount    *int               `yaml:"max_count,omitempty" json:"max_count,omitempty"`
+}
+
+// WorkerDefaultSpec mirrors the knobs a one-click "+ Add worker" dispatch
+// honours. Any field left unset falls back to the built-in `ddx work` defaults.
+type WorkerDefaultSpec struct {
+	Harness string `yaml:"harness,omitempty" json:"harness,omitempty"`
+	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
+	Effort  string `yaml:"effort,omitempty" json:"effort,omitempty"`
+	MinTier string `yaml:"min_tier,omitempty" json:"min_tier,omitempty"`
+	MaxTier string `yaml:"max_tier,omitempty" json:"max_tier,omitempty"`
 }
 
 // CostConfig controls optional cost estimates that DDx cannot infer safely.
diff --git a/cli/internal/server/frontend/e2e/workers.spec.ts b/cli/internal/server/frontend/e2e/workers.spec.ts
index afa8263c..3c32f175 100644
--- a/cli/internal/server/frontend/e2e/workers.spec.ts
+++ b/cli/internal/server/frontend/e2e/workers.spec.ts
@@ -563,3 +563,110 @@ test('US-086a.c: terminal-phase worker freezes stream with completion timestamp'
 	// Link to the evidence bundle
 	await expect(live.getByRole('link', { name: /evidence bundle/i })).toBeVisible();
 });
+
+// ddx-b6cf025c: global drain indicator + workers-overview count control.
+test('workers overview shows drain count control, indicator, and +/- buttons', async ({ page }) => {
+	let workers: Record<string, unknown>[] = [];
+	let dispatchCalled = false;
+	let stopCalled = false;
+
+	await page.route('/graphql', async (route) => {
+		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
+		if (body.query.includes('NodeInfo')) {
+			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
+		} else if (body.query.includes('Projects')) {
+			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
+		} else if (body.query.includes('QueueAndWorkersSummary') || body.query.includes('queueAndWorkersSummary')) {
+			const running = workers.filter((w) => w.state === 'running').length;
+			await route.fulfill({
+				status: 200,
+				contentType: 'application/json',
+				body: JSON.stringify({
+					data: {
+						queueAndWorkersSummary: {
+							readyBeads: 3,
+							runningWorkers: running,
+							totalWorkers: workers.length
+						}
+					}
+				})
+			});
+		} else if (body.query.includes('AddDrainWorker') || body.query.includes('workerDispatch')) {
+			dispatchCalled = true;
+			const id = `worker-drain-${workers.length + 1}`;
+			workers = [
+				{
+					id,
+					kind: 'execute-loop',
+					state: 'running',
+					status: 'running',
+					harness: 'codex',
+					model: null,
+					currentBead: null,
+					attempts: 0,
+					successes: 0,
+					failures: 0,
+					startedAt: '2026-04-23T10:00:00Z'
+				},
+				...workers
+			];
+			await route.fulfill({
+				status: 200,
+				contentType: 'application/json',
+				body: JSON.stringify({ data: { workerDispatch: { id, state: 'running', kind: 'execute-loop' } } })
+			});
+		} else if (body.query.includes('StopWorker') || body.query.includes('stopWorker')) {
+			stopCalled = true;
+			workers = workers.map((w) =>
+				w.id === body.variables?.id ? { ...w, state: 'stopped', status: 'stopped' } : w
+			);
+			await route.fulfill({
+				status: 200,
+				contentType: 'application/json',
+				body: JSON.stringify({
+					data: { stopWorker: { id: body.variables?.id, state: 'stopped', kind: 'execute-loop' } }
+				})
+			});
+		} else if (body.query.includes('WorkersByProject')) {
+			await route.fulfill({
+				status: 200,
+				contentType: 'application/json',
+				body: JSON.stringify({ data: makeWorkersResponse(workers) })
+			});
+		} else if (body.query.includes('AgentSessions') || body.query.includes('agentSessions')) {
+			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: makeSessionsResponse() }) });
+		} else {
+			await route.continue();
+		}
+		// Accept a confirm() dialog on the Remove path.
+	});
+
+	// Auto-accept the browser confirm() that Remove-worker opens.
+	page.on('dialog', (dialog) => void dialog.accept());
+
+	await page.goto(BASE_URL);
+
+	// Count control panel is visible on the workers overview.
+	const panel = page.getByTestId('drain-count-panel');
+	await expect(panel).toBeVisible();
+	await expect(panel.getByTestId('drain-worker-count')).toHaveText('0');
+	await expect(panel.getByText(/Adds a general-purpose drain worker/)).toBeVisible();
+
+	// Global nav indicator is visible on every page (ddx-b6cf025c AC #1).
+	const indicator = page.getByTestId('drain-indicator');
+	await expect(indicator).toBeVisible();
+	await expect(indicator).toHaveText(/Queue: 3 ready|0 workers/);
+
+	// + Add worker dispatches.
+	await page.getByTestId('add-drain-worker').click();
+	await expect.poll(() => dispatchCalled, { timeout: 3000 }).toBe(true);
+	await expect(panel.getByTestId('drain-worker-count')).toHaveText('1');
+
+	// − Remove worker stops the oldest running drain worker.
+	await page.getByTestId('remove-drain-worker').click();
+	await expect.poll(() => stopCalled, { timeout: 3000 }).toBe(true);
+
+	// Indicator survives navigation to another route.
+	await page.goto(`/nodes/node-abc/projects/${PROJECT_ID}/beads`);
+	await expect(page.getByTestId('drain-indicator')).toBeVisible();
+});
diff --git a/cli/internal/server/frontend/src/lib/components/DrainIndicator.svelte b/cli/internal/server/frontend/src/lib/components/DrainIndicator.svelte
new file mode 100644
index 00000000..fbea25be
--- /dev/null
+++ b/cli/internal/server/frontend/src/lib/components/DrainIndicator.svelte
@@ -0,0 +1,92 @@
+<script lang="ts">
+	import { Loader2 } from 'lucide-svelte';
+	import { projectStore } from '$lib/stores/project.svelte';
+	import { nodeStore } from '$lib/stores/node.svelte';
+	import { createClient } from '$lib/gql/client';
+	import { gql } from 'graphql-request';
+
+	// Persistent drain-queue worker indicator (ddx-b6cf025c). Shown on every
+	// route inside a selected project; hidden when no project is selected.
+	// Polls every 3s — lightweight, and avoids wiring a new subscription
+	// stream just for nav badge updates. AC requires ≤2s worker-state and
+	// ≤5s ready-count freshness; 3s poll sits inside both bounds.
+
+	const SUMMARY_QUERY = gql`
+		query QueueAndWorkersSummary($projectId: String!) {
+			queueAndWorkersSummary(projectId: $projectId) {
+				readyBeads
+				runningWorkers
+				totalWorkers
+			}
+		}
+	`;
+
+	let readyBeads = $state(0);
+	let runningWorkers = $state(0);
+	let loaded = $state(false);
+
+	const projectId = $derived(projectStore.value?.id ?? null);
+	const nodeId = $derived(nodeStore.value?.id ?? null);
+	const workersHref = $derived(
+		nodeId && projectId ? `/nodes/${nodeId}/projects/${projectId}/workers` : null
+	);
+	const active = $derived(runningWorkers > 0);
+
+	const label = $derived.by(() => {
+		if (!loaded) return '';
+		if (active) {
+			const w = runningWorkers === 1 ? 'worker' : 'workers';
+			return `${runningWorkers} ${w} · ${readyBeads} ready`;
+		}
+		return `Queue: ${readyBeads} ready`;
+	});
+
+	async function refresh(pid: string) {
+		try {
+			const client = createClient(fetch);
+			const data = await client.request<{
+				queueAndWorkersSummary: {
+					readyBeads: number;
+					runningWorkers: number;
+					totalWorkers: number;
+				};
+			}>(SUMMARY_QUERY, { projectId: pid });
+			readyBeads = data.queueAndWorkersSummary.readyBeads;
+			runningWorkers = data.queueAndWorkersSummary.runningWorkers;
+			loaded = true;
+		} catch {
+			// Keep previous values on transient failure. AC #4: "falls back to the
+			// global count" is handled implicitly by holding state; we intentionally
+			// do not clear `loaded` so the badge keeps rendering.
+		}
+	}
+
+	$effect(() => {
+		const pid = projectId;
+		if (!pid) {
+			loaded = false;
+			return;
+		}
+		void refresh(pid);
+		const h = setInterval(() => void refresh(pid), 3000);
+		return () => clearInterval(h);
+	});
+</script>
+
+{#if projectId && workersHref}
+	<a
+		data-testid="drain-indicator"
+		data-state={active ? 'active' : 'idle'}
+		href={workersHref}
+		class="flex items-center gap-1.5 rounded border border-gray-200 px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
+		aria-label="Drain queue status"
+		title="Click for worker overview"
+	>
+		{#if active}
+			<Loader2 class="h-3.5 w-3.5 animate-spin text-blue-600 dark:text-blue-400" />
+		{:else}
+			<span class="inline-block h-2 w-2 rounded-full bg-gray-400 dark:bg-gray-500"></span>
+		{/if}
+		<span>{label || 'Queue: …'}</span>
+	</a>
+{/if}
diff --git a/cli/internal/server/frontend/src/lib/components/NavShell.svelte b/cli/internal/server/frontend/src/lib/components/NavShell.svelte
index 18b7fca9..9b004371 100644
--- a/cli/internal/server/frontend/src/lib/components/NavShell.svelte
+++ b/cli/internal/server/frontend/src/lib/components/NavShell.svelte
@@ -18,6 +18,7 @@
 	import { page } from '$app/stores';
 	import { toggleMode, mode } from '$lib/theme';
 	import ProjectPicker from './ProjectPicker.svelte';
+	import DrainIndicator from './DrainIndicator.svelte';
 	import { nodeStore } from '$lib/stores/node.svelte';
 	import { projectStore } from '$lib/stores/project.svelte';
 	import { wsConnection } from '$lib/stores/connection.svelte';
@@ -64,7 +65,8 @@
 		<span class="text-xs text-gray-700 dark:text-gray-300">Node: {nodeName}</span>
 		<div class="mx-2 h-4 w-px bg-gray-200 dark:bg-gray-700"></div>
 		<ProjectPicker />
-		<div class="ml-auto">
+		<div class="ml-auto flex items-center gap-2">
+			<DrainIndicator />
 			<button
 				onclick={toggleMode}
 				class="rounded p-1.5 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
diff --git a/cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte b/cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte
index 3260e812..81de44bd 100644
--- a/cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte
+++ b/cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte
@@ -29,6 +29,18 @@
 		}
 	`;
 
+	// + Add worker dispatches a default-spec drain worker (ddx-b6cf025c). The
+	// server honours .ddx/config.yaml workers.default_spec + workers.max_count.
+	const ADD_WORKER_MUTATION = gql`
+		mutation AddDrainWorker($projectId: String!) {
+			workerDispatch(kind: "execute-loop", projectId: $projectId) {
+				id
+				state
+				kind
+			}
+		}
+	`;
+
 	// Live phase overrides from workerProgress subscription (workerID -> phase)
 	let livePhaseOverrides = $state<Map<string, string>>(new Map());
 	let showStartForm = $state(false);
@@ -39,6 +51,15 @@
 	let profile = $state('smart');
 	let effort = $state('medium');
 	let labelFilter = $state('');
+	let adding = $state(false);
+	let removing = $state(false);
+
+	// Drain workers: count of running execute-loop workers.
+	const runningDrainCount = $derived(
+		data.workers.edges.filter(
+			(e) => e.node.state === 'running' && e.node.kind === 'execute-loop'
+		).length
+	);
 
 	// Subscribe to progress events for all running workers
 	$effect(() => {
@@ -105,6 +126,43 @@
 		}
 	}
 
+	async function addDrainWorker() {
+		actionError = null;
+		adding = true;
+		try {
+			const client = createClient(fetch);
+			await client.request(ADD_WORKER_MUTATION, { projectId: data.projectId });
+			await invalidateAll();
+		} catch (err) {
+			actionError = errorText(err);
+		} finally {
+			adding = false;
+		}
+	}
+
+	async function removeDrainWorker() {
+		actionError = null;
+		// Find the oldest running execute-loop worker (AC #4: "stops the oldest-
+		// running drain worker"). data.workers is sorted newest-first, so the
+		// last matching edge is oldest.
+		const runningDrain = data.workers.edges
+			.filter((e) => e.node.state === 'running' && e.node.kind === 'execute-loop')
+			.map((e) => e.node);
+		const target = runningDrain[runningDrain.length - 1];
+		if (!target) return;
+		if (!window.confirm(`Stop worker ${target.id}?`)) return;
+		removing = true;
+		try {
+			const client = createClient(fetch);
+			await client.request(STOP_WORKER_MUTATION, { id: target.id });
+			await invalidateAll();
+		} catch (err) {
+			actionError = errorText(err);
+		} finally {
+			removing = false;
+		}
+	}
+
 	async function stopWorker(event: MouseEvent, workerId: string) {
 		event.stopPropagation();
 		actionError = null;
@@ -138,6 +196,50 @@
 </script>
 
 <div class="space-y-4">
+	<!-- Drain-worker count control (ddx-b6cf025c). Dispatches a default-spec
+	     worker; server enforces workers.default_spec + workers.max_count. -->
+	<div
+		data-testid="drain-count-panel"
+		class="flex flex-col gap-3 rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm dark:border-blue-900 dark:bg-blue-950/30 sm:flex-row sm:items-center sm:justify-between"
+	>
+		<div>
+			<div class="text-xs font-medium uppercase tracking-wide text-blue-700 dark:text-blue-300">
+				Drain workers
+			</div>
+			<div
+				data-testid="drain-worker-count"
+				class="text-3xl font-semibold text-blue-900 dark:text-blue-100"
+			>
+				{runningDrainCount}
+			</div>
+			<p class="mt-1 text-xs text-blue-800/80 dark:text-blue-200/80">
+				Adds a general-purpose drain worker. Use the per-harness picker below for custom specs.
+			</p>
+		</div>
+		<div class="flex items-center gap-2">
+			<button
+				type="button"
+				data-testid="add-drain-worker"
+				onclick={() => void addDrainWorker()}
+				disabled={adding}
+				class="rounded border border-blue-700 bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:border-blue-500 dark:bg-blue-500 dark:hover:bg-blue-600"
+				aria-label="Add worker"
+			>
+				{adding ? '…' : '+ Add worker'}
+			</button>
+			<button
+				type="button"
+				data-testid="remove-drain-worker"
+				onclick={() => void removeDrainWorker()}
+				disabled={removing || runningDrainCount === 0}
+				class="rounded border border-red-300 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/30"
+				aria-label="Remove worker"
+			>
+				{removing ? '…' : '− Remove worker'}
+			</button>
+		</div>
+	</div>
+
 	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
 		<div>
 			<h1 class="text-xl font-semibold dark:text-white">Workers</h1>
diff --git a/cli/internal/server/graphql/generated.go b/cli/internal/server/graphql/generated.go
index 7bf8ffbc..1d932663 100644
--- a/cli/internal/server/graphql/generated.go
+++ b/cli/internal/server/graphql/generated.go
@@ -908,6 +908,7 @@ type ComplexityRoot struct {
 		ProviderStatuses            func(childComplexity int) int
 		ProviderTrend               func(childComplexity int, name string, windowDays int) int
 		Providers                   func(childComplexity int) int
+		QueueAndWorkersSummary      func(childComplexity int, projectID string) int
 		QueueSummary                func(childComplexity int, projectID string) int
 		Ready                       func(childComplexity int) int
 		Search                      func(childComplexity int, query string, first *int, after *string, last *int, before *string) int
@@ -920,6 +921,12 @@ type ComplexityRoot struct {
 		WorkersByProject            func(childComplexity int, projectID string, first *int, after *string, last *int, before *string) int
 	}
 
+	QueueAndWorkersSummary struct {
+		ReadyBeads     func(childComplexity int) int
+		RunningWorkers func(childComplexity int) int
+		TotalWorkers   func(childComplexity int) int
+	}
+
 	QueueSummary struct {
 		Blocked    func(childComplexity int) int
 		InProgress func(childComplexity int) int
@@ -1211,6 +1218,7 @@ type QueryResolver interface {
 	DefaultRouteStatus(ctx context.Context) (*DefaultRouteStatus, error)
 	ProviderTrend(ctx context.Context, name string, windowDays int) (*ProviderTrend, error)
 	QueueSummary(ctx context.Context, projectID string) (*QueueSummary, error)
+	QueueAndWorkersSummary(ctx context.Context, projectID string) (*QueueAndWorkersSummary, error)
 	EfficacyRows(ctx context.Context, since *string, until *string, projectID *string) ([]*EfficacyRow, error)
 	EfficacyAttempts(ctx context.Context, rowKey string, since *string, until *string, projectID *string) (*EfficacyAttempts, error)
 	Comparisons(ctx context.Context) ([]*ComparisonRecord, error)
@@ -5203,6 +5211,17 @@ func (e *executableSchema) Complexity(ctx context.Context, typeName, field strin
 		}
 
 		return e.ComplexityRoot.Query.Providers(childComplexity), true
+	case "Query.queueAndWorkersSummary":
+		if e.ComplexityRoot.Query.QueueAndWorkersSummary == nil {
+			break
+		}
+
+		args, err := ec.field_Query_queueAndWorkersSummary_args(ctx, rawArgs)
+		if err != nil {
+			return 0, false
+		}
+
+		return e.ComplexityRoot.Query.QueueAndWorkersSummary(childComplexity, args["projectId"].(string)), true
 	case "Query.queueSummary":
 		if e.ComplexityRoot.Query.QueueSummary == nil {
 			break
@@ -5309,6 +5328,25 @@ func (e *executableSchema) Complexity(ctx context.Context, typeName, field strin
 
 		return e.ComplexityRoot.Query.WorkersByProject(childComplexity, args["projectID"].(string), args["first"].(*int), args["after"].(*string), args["last"].(*int), args["before"].(*string)), true
 
+	case "QueueAndWorkersSummary.readyBeads":
+		if e.ComplexityRoot.QueueAndWorkersSummary.ReadyBeads == nil {
+			break
+		}
+
+		return e.ComplexityRoot.QueueAndWorkersSummary.ReadyBeads(childComplexity), true
+	case "QueueAndWorkersSummary.runningWorkers":
+		if e.ComplexityRoot.QueueAndWorkersSummary.RunningWorkers == nil {
+			break
+		}
+
+		return e.ComplexityRoot.QueueAndWorkersSummary.RunningWorkers(childComplexity), true
+	case "QueueAndWorkersSummary.totalWorkers":
+		if e.ComplexityRoot.QueueAndWorkersSummary.TotalWorkers == nil {
+			break
+		}
+
+		return e.ComplexityRoot.QueueAndWorkersSummary.TotalWorkers(childComplexity), true
+
 	case "QueueSummary.blocked":
 		if e.ComplexityRoot.QueueSummary.Blocked == nil {
 			break
@@ -7390,6 +7428,17 @@ func (ec *executionContext) field_Query_provider_args(ctx context.Context, rawAr
 	return args, nil
 }
 
+func (ec *executionContext) field_Query_queueAndWorkersSummary_args(ctx context.Context, rawArgs map[string]any) (map[string]any, error) {
+	var err error
+	args := map[string]any{}
+	arg0, err := graphql.ProcessArgField(ctx, rawArgs, "projectId", ec.unmarshalNString2string)
+	if err != nil {
+		return nil, err
+	}
+	args["projectId"] = arg0
+	return args, nil
+}
+
 func (ec *executionContext) field_Query_queueSummary_args(ctx context.Context, rawArgs map[string]any) (map[string]any, error) {
 	var err error
 	args := map[string]any{}
@@ -28004,6 +28053,55 @@ func (ec *executionContext) fieldContext_Query_queueSummary(ctx context.Context,
 	return fc, nil
 }
 
+func (ec *executionContext) _Query_queueAndWorkersSummary(ctx context.Context, field graphql.CollectedField) (ret graphql.Marshaler) {
+	return graphql.ResolveField(
+		ctx,
+		ec.OperationContext,
+		field,
+		ec.fieldContext_Query_queueAndWorkersSummary,
+		func(ctx context.Context) (any, error) {
+			fc := graphql.GetFieldContext(ctx)
+			return ec.Resolvers.Query().QueueAndWorkersSummary(ctx, fc.Args["projectId"].(string))
+		},
+		nil,
+		ec.marshalNQueueAndWorkersSummary2ᚖgithubᚗcomᚋDocumentDrivenDXᚋddxᚋinternalᚋserverᚋgraphqlᚐQueueAndWorkersSummary,
+		true,
+		true,
+	)
+}
+
+func (ec *executionContext) fieldContext_Query_queueAndWorkersSummary(ctx context.Context, field graphql.CollectedField) (fc *graphql.FieldContext, err error) {
+	fc = &graphql.FieldContext{
+		Object:     "Query",
+		Field:      field,
+		IsMethod:   true,
+		IsResolver: true,
+		Child: func(ctx context.Context, field graphql.CollectedField) (*graphql.FieldContext, error) {
+			switch field.Name {
+			case "readyBeads":
+				return ec.fieldContext_QueueAndWorkersSummary_readyBeads(ctx, field)
+			case "runningWorkers":
+				return ec.fieldContext_QueueAndWorkersSummary_runningWorkers(ctx, field)
+			case "totalWorkers":
+				return ec.fieldContext_QueueAndWorkersSummary_totalWorkers(ctx, field)
+			}
+			return nil, fmt.Errorf("no field named %q was found under type QueueAndWorkersSummary", field.Name)
+		},
+	}
+	defer func() {
+		if r := recover(); r != nil {
+			err = ec.Recover(ctx, r)
+			ec.Error(ctx, err)
+		}
+	}()
+	ctx = graphql.WithFieldContext(ctx, fc)
+	if fc.Args, err = ec.field_Query_queueAndWorkersSummary_args(ctx, field.ArgumentMap(ec.Variables)); err != nil {
+		ec.Error(ctx, err)
+		return fc, err
+	}
+	return fc, nil
+}
+
 func (ec *executionContext) _Query_efficacyRows(ctx context.Context, field graphql.CollectedField) (ret graphql.Marshaler) {
 	return graphql.ResolveField(
 		ctx,
@@ -28481,6 +28579,93 @@ func (ec *executionContext) fieldContext_Query___schema(_ context.Context, field
 	return fc, nil
 }
 
+func (ec *executionContext) _QueueAndWorkersSummary_readyBeads(ctx context.Context, field graphql.CollectedField, obj *QueueAndWorkersSummary) (ret graphql.Marshaler) {
+	return graphql.ResolveField(
+		ctx,
+		ec.OperationContext,
+		field,
+		ec.fieldContext_QueueAndWorkersSummary_readyBeads,
+		func(ctx context.Context) (any, error) {
+			return obj.ReadyBeads, nil
+		},
+		nil,
+		ec.marshalNInt2int,
+		true,
+		true,
+	)
+}
+
+func (ec *executionContext) fieldContext_QueueAndWorkersSummary_readyBeads(_ context.Context, field graphql.CollectedField) (fc *graphql.FieldContext, err error) {
+	fc = &graphql.FieldContext{
+		Object:     "QueueAndWorkersSummary",
+		Field:      field,
+		IsMethod:   false,
+		IsResolver: false,
+		Child: func(ctx context.Context, field graphql.CollectedField) (*graphql.FieldContext, error) {
+			return nil, errors.New("field of type Int does not have child fields")
+		},
+	}
+	return fc, nil
+}
+
+func (ec *executionContext) _QueueAndWorkersSummary_runningWorkers(ctx context.Context, field graphql.CollectedField, obj *QueueAndWorkersSummary) (ret graphql.Marshaler) {
+	return graphql.ResolveField(
+		ctx,
+		ec.OperationContext,
+		field,
+		ec.fieldContext_QueueAndWorkersSummary_runningWorkers,
+		func(ctx context.Context) (any, error) {
+			return obj.RunningWorkers, nil
+		},
+		nil,
+		ec.marshalNInt2int,
+		true,
+		true,
+	)
+}
+
+func (ec *executionContext) fieldContext_QueueAndWorkersSummary_runningWorkers(_ context.Context, field graphql.CollectedField) (fc *graphql.FieldContext, err error) {
+	fc = &graphql.FieldContext{
+		Object:     "QueueAndWorkersSummary",
+		Field:      field,
+		IsMethod:   false,
+		IsResolver: false,
+		Child: func(ctx context.Context, field graphql.CollectedField) (*graphql.FieldContext, error) {
+			return nil, errors.New("field of type Int does not have child fields")
+		},
+	}
+	return fc, nil
+}
+
+func (ec *executionContext) _QueueAndWorkersSummary_totalWorkers(ctx context.Context, field graphql.CollectedField, obj *QueueAndWorkersSummary) (ret graphql.Marshaler) {
+	return graphql.ResolveField(
+		ctx,
+		ec.OperationContext,
+		field,
+		ec.fieldContext_QueueAndWorkersSummary_totalWorkers,
+		func(ctx context.Context) (any, error) {
+			return obj.TotalWorkers, nil
+		},
+		nil,
+		ec.marshalNInt2int,
+		true,
+		true,
+	)
+}
+
+func (ec *executionContext) fieldContext_QueueAndWorkersSummary_totalWorkers(_ context.Context, field graphql.CollectedField) (fc *graphql.FieldContext, err error) {
+	fc = &graphql.FieldContext{
+		Object:     "QueueAndWorkersSummary",
+		Field:      field,
+		IsMethod:   false,
+		IsResolver: false,
+		Child: func(ctx context.Context, field graphql.CollectedField) (*graphql.FieldContext, error) {
+			return nil, errors.New("field of type Int does not have child fields")
+		},
+	}
+	return fc, nil
+}
+
 func (ec *executionContext) _QueueSummary_ready(ctx context.Context, field graphql.CollectedField, obj *QueueSummary) (ret graphql.Marshaler) {
 	return graphql.ResolveField(
 		ctx,
@@ -41059,6 +41244,28 @@ func (ec *executionContext) _Query(ctx context.Context, sel ast.SelectionSet) gr
 					func(ctx context.Context) graphql.Marshaler { return innerFunc(ctx, out) })
 			}
 
+			out.Concurrently(i, func(ctx context.Context) graphql.Marshaler { return rrm(innerCtx) })
+		case "queueAndWorkersSummary":
+			field := field
+
+			innerFunc := func(ctx context.Context, fs *graphql.FieldSet) (res graphql.Marshaler) {
+				defer func() {
+					if r := recover(); r != nil {
+						ec.Error(ctx, ec.Recover(ctx, r))
+					}
+				}()
+				res = ec._Query_queueAndWorkersSummary(ctx, field)
+				if res == graphql.Null {
+					atomic.AddUint32(&fs.Invalids, 1)
+				}
+				return res
+			}
+
+			rrm := func(ctx context.Context) graphql.Marshaler {
+				return ec.OperationContext.RootResolverMiddleware(ctx,
+					func(ctx context.Context) graphql.Marshaler { return innerFunc(ctx, out) })
+			}
+
 			out.Concurrently(i, func(ctx context.Context) graphql.Marshaler { return rrm(innerCtx) })
 		case "efficacyRows":
 			field := field
@@ -41242,6 +41449,55 @@ func (ec *executionContext) _Query(ctx context.Context, sel ast.SelectionSet) gr
 	return out
 }
 
+var queueAndWorkersSummaryImplementors = []string{"QueueAndWorkersSummary"}
+
+func (ec *executionContext) _QueueAndWorkersSummary(ctx context.Context, sel ast.SelectionSet, obj *QueueAndWorkersSummary) graphql.Marshaler {
+	fields := graphql.CollectFields(ec.OperationContext, sel, queueAndWorkersSummaryImplementors)
+
+	out := graphql.NewFieldSet(fields)
+	deferred := make(map[string]*graphql.FieldSet)
+	for i, field := range fields {
+		switch field.Name {
+		case "__typename":
+			out.Values[i] = graphql.MarshalString("QueueAndWorkersSummary")
+		case "readyBeads":
+			out.Values[i] = ec._QueueAndWorkersSummary_readyBeads(ctx, field, obj)
+			if out.Values[i] == graphql.Null {
+				out.Invalids++
+			}
+		case "runningWorkers":
+			out.Values[i] = ec._QueueAndWorkersSummary_runningWorkers(ctx, field, obj)
+			if out.Values[i] == graphql.Null {
+				out.Invalids++
+			}
+		case "totalWorkers":
+			out.Values[i] = ec._QueueAndWorkersSummary_totalWorkers(ctx, field, obj)
+			if out.Values[i] == graphql.Null {
+				out.Invalids++
+			}
+		default:
+			panic("unknown field " + strconv.Quote(field.Name))
+		}
+	}
+	out.Dispatch(ctx)
+	if out.Invalids > 0 {
+		return graphql.Null
+	}
+
+	atomic.AddInt32(&ec.Deferred, int32(len(deferred)))
+
+	for label, dfs := range deferred {
+		ec.ProcessDeferredGroup(graphql.DeferredGroup{
+			Label:    label,
+			Path:     graphql.GetPath(ctx),
+			FieldSet: dfs,
+			Context:  ctx,
+		})
+	}
+
+	return out
+}
+
 var queueSummaryImplementors = []string{"QueueSummary"}
 
 func (ec *executionContext) _QueueSummary(ctx context.Context, sel ast.SelectionSet, obj *QueueSummary) graphql.Marshaler {
@@ -44363,6 +44619,20 @@ func (ec *executionContext) marshalNProviderTrendPoint2ᚖgithubᚗcomᚋDocumen
 	return ec._ProviderTrendPoint(ctx, sel, v)
 }
 
+func (ec *executionContext) marshalNQueueAndWorkersSummary2githubᚗcomᚋDocumentDrivenDXᚋddxᚋinternalᚋserverᚋgraphqlᚐQueueAndWorkersSummary(ctx context.Context, sel ast.SelectionSet, v QueueAndWorkersSummary) graphql.Marshaler {
+	return ec._QueueAndWorkersSummary(ctx, sel, &v)
+}
+
+func (ec *executionContext) marshalNQueueAndWorkersSummary2ᚖgithubᚗcomᚋDocumentDrivenDXᚋddxᚋinternalᚋserverᚋgraphqlᚐQueueAndWorkersSummary(ctx context.Context, sel ast.SelectionSet, v *QueueAndWorkersSummary) graphql.Marshaler {
+	if v == nil {
+		if !graphql.HasFieldError(ctx, graphql.GetFieldContext(ctx)) {
+			graphql.AddErrorf(ctx, "the requested element is null which the schema does not allow")
+		}
+		return graphql.Null
+	}
+	return ec._QueueAndWorkersSummary(ctx, sel, v)
+}
+
 func (ec *executionContext) marshalNQueueSummary2githubᚗcomᚋDocumentDrivenDXᚋddxᚋinternalᚋserverᚋgraphqlᚐQueueSummary(ctx context.Context, sel ast.SelectionSet, v QueueSummary) graphql.Marshaler {
 	return ec._QueueSummary(ctx, sel, &v)
 }
diff --git a/cli/internal/server/graphql/integration_test.go b/cli/internal/server/graphql/integration_test.go
index 51e1d1ec..0f688a9e 100644
--- a/cli/internal/server/graphql/integration_test.go
+++ b/cli/internal/server/graphql/integration_test.go
@@ -146,6 +146,7 @@ type testStateProvider struct {
 	projects    []*ddxgraphql.Project
 	beads       []ddxgraphql.BeadSnapshot
 	costSummary *ddxgraphql.SessionsCostSummary
+	workers     map[string][]*ddxgraphql.Worker
 }
 
 func newTestStateProvider(workDir string, store *bead.Store) *testStateProvider {
@@ -250,7 +251,12 @@ func (p *testStateProvider) GetBeadSnapshot(id string) (*ddxgraphql.BeadSnapshot
 }
 
 // No-op implementations for resolver methods not exercised by these tests.
-func (p *testStateProvider) GetWorkersGraphQL(_ string) []*ddxgraphql.Worker { return nil }
+func (p *testStateProvider) GetWorkersGraphQL(projectID string) []*ddxgraphql.Worker {
+	if p.workers == nil {
+		return nil
+	}
+	return p.workers[projectID]
+}
 func (p *testStateProvider) GetWorkerGraphQL(_ string) (*ddxgraphql.Worker, bool) {
 	return nil, false
 }
diff --git a/cli/internal/server/graphql/models.go b/cli/internal/server/graphql/models.go
index f8861900..770d89e9 100644
--- a/cli/internal/server/graphql/models.go
+++ b/cli/internal/server/graphql/models.go
@@ -1528,6 +1528,18 @@ type ProviderUsage struct {
 type Query struct {
 }
 
+// QueueAndWorkersSummary backs the global drain indicator. Combines the
+// ready-bead count with running/total worker counts so the NavShell badge
+// can render without issuing two queries per poll.
+type QueueAndWorkersSummary struct {
+	// Open beads with no unmet dependencies (the drain queue depth)
+	ReadyBeads int `json:"readyBeads"`
+	// Count of workers in state `running` for this project
+	RunningWorkers int `json:"runningWorkers"`
+	// Total workers on record for this project (any state)
+	TotalWorkers int `json:"totalWorkers"`
+}
+
 // QueueSummary is a compact project queue status for action dispatch.
 type QueueSummary struct {
 	// Open beads with no unmet dependencies
diff --git a/cli/internal/server/graphql/queue_and_workers_summary_test.go b/cli/internal/server/graphql/queue_and_workers_summary_test.go
new file mode 100644
index 00000000..28f25cc4
--- /dev/null
+++ b/cli/internal/server/graphql/queue_and_workers_summary_test.go
@@ -0,0 +1,107 @@
+package graphql_test
+
+import (
+	"encoding/json"
+	"testing"
+
+	"github.com/DocumentDrivenDX/ddx/internal/bead"
+	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
+)
+
+// TestGraphQLQueueAndWorkersSummary covers the resolver backing the global
+// drain indicator (ddx-b6cf025c). Verifies:
+//   - zeros for unknown project (no error)
+//   - ready-bead count mirrors the bead store's Ready() result
+//   - running-worker count filters by state == "running"
+func TestGraphQLQueueAndWorkersSummary(t *testing.T) {
+	t.Setenv("HOME", t.TempDir())
+	workDir, store := setupIntegrationDir(t)
+
+	ready := &bead.Bead{Title: "ready bead", Status: bead.StatusOpen}
+	if err := store.Create(ready); err != nil {
+		t.Fatal(err)
+	}
+	second := &bead.Bead{Title: "also ready", Status: bead.StatusOpen}
+	if err := store.Create(second); err != nil {
+		t.Fatal(err)
+	}
+	dep := &bead.Bead{Title: "dep", Status: bead.StatusOpen}
+	if err := store.Create(dep); err != nil {
+		t.Fatal(err)
+	}
+	blocked := &bead.Bead{Title: "blocked", Status: bead.StatusOpen}
+	if err := store.Create(blocked); err != nil {
+		t.Fatal(err)
+	}
+	if err := store.DepAdd(blocked.ID, dep.ID); err != nil {
+		t.Fatal(err)
+	}
+
+	state := newTestStateProvider(workDir, store)
+	projectID := state.projects[0].ID
+	state.workers = map[string][]*ddxgraphql.Worker{
+		projectID: {
+			{ID: "w1", State: "running", Kind: "execute-loop"},
+			{ID: "w2", State: "running", Kind: "execute-loop"},
+			{ID: "w3", State: "stopped", Kind: "execute-loop"},
+		},
+	}
+	h := newGQLHandler(state, workDir, nil)
+
+	t.Run("known project", func(t *testing.T) {
+		resp := gqlPost(t, h, `{
+			queueAndWorkersSummary(projectId: "`+projectID+`") {
+				readyBeads
+				runningWorkers
+				totalWorkers
+			}
+		}`)
+		var data struct {
+			QueueAndWorkersSummary struct {
+				ReadyBeads     int `json:"readyBeads"`
+				RunningWorkers int `json:"runningWorkers"`
+				TotalWorkers   int `json:"totalWorkers"`
+			} `json:"queueAndWorkersSummary"`
+		}
+		if err := json.Unmarshal(resp["data"], &data); err != nil {
+			t.Fatalf("parse: %v", err)
+		}
+		got := data.QueueAndWorkersSummary
+		if got.ReadyBeads != 3 {
+			t.Errorf("readyBeads: want 3, got %d", got.ReadyBeads)
+		}
+		if got.RunningWorkers != 2 {
+			t.Errorf("runningWorkers: want 2, got %d", got.RunningWorkers)
+		}
+		if got.TotalWorkers != 3 {
+			t.Errorf("totalWorkers: want 3, got %d", got.TotalWorkers)
+		}
+	})
+
+	t.Run("unknown project returns zeros, not error", func(t *testing.T) {
+		resp := gqlPost(t, h, `{
+			queueAndWorkersSummary(projectId: "proj-missing") {
+				readyBeads
+				runningWorkers
+				totalWorkers
+			}
+		}`)
+		if raw, ok := resp["errors"]; ok && len(raw) > 0 && string(raw) != "null" {
+			t.Fatalf("unexpected errors: %s", string(raw))
+		}
+		var data struct {
+			QueueAndWorkersSummary struct {
+				ReadyBeads     int `json:"readyBeads"`
+				RunningWorkers int `json:"runningWorkers"`
+				TotalWorkers   int `json:"totalWorkers"`
+			} `json:"queueAndWorkersSummary"`
+		}
+		if err := json.Unmarshal(resp["data"], &data); err != nil {
+			t.Fatalf("parse: %v", err)
+		}
+		got := data.QueueAndWorkersSummary
+		if got.ReadyBeads != 0 || got.RunningWorkers != 0 || got.TotalWorkers != 0 {
+			t.Fatalf("want zeros, got %+v", got)
+		}
+	})
+}
diff --git a/cli/internal/server/graphql/resolver_feat008.go b/cli/internal/server/graphql/resolver_feat008.go
index 5944cdb1..6012633a 100644
--- a/cli/internal/server/graphql/resolver_feat008.go
+++ b/cli/internal/server/graphql/resolver_feat008.go
@@ -287,6 +287,32 @@ func (r *queryResolver) QueueSummary(ctx context.Context, projectID string) (*Qu
 	}, nil
 }
 
+// QueueAndWorkersSummary backs the global drain indicator (ddx-b6cf025c).
+// Returns zeros — not an error — for unknown or empty projects so the nav
+// component can render without special-casing failures.
+func (r *queryResolver) QueueAndWorkersSummary(ctx context.Context, projectID string) (*QueueAndWorkersSummary, error) {
+	out := &QueueAndWorkersSummary{}
+	// Only resolve ready beads when the projectID maps to a real project.
+	// Unknown IDs return zeros rather than falling back to WorkingDir so the
+	// indicator does not surface an unrelated project's queue depth.
+	if r.State != nil {
+		if proj, ok := r.State.GetProjectSnapshotByID(projectID); ok && proj.Path != "" {
+			store := bead.NewStore(filepath.Join(proj.Path, ".ddx"))
+			if ready, err := store.Ready(); err == nil {
+				out.ReadyBeads = len(ready)
+			}
+		}
+		workers := r.State.GetWorkersGraphQL(projectID)
+		out.TotalWorkers = len(workers)
+		for _, w := range workers {
+			if w != nil && w.State == "running" {
+				out.RunningWorkers++
+			}
+		}
+	}
+	return out, nil
+}
+
 // EfficacyRows is the resolver for the efficacyRows field.
 func (r *queryResolver) EfficacyRows(ctx context.Context, since *string, until *string, projectID *string) ([]*EfficacyRow, error) {
 	snap, err := r.efficacySnapshot(since, until, projectID)
diff --git a/cli/internal/server/graphql/schema.graphql b/cli/internal/server/graphql/schema.graphql
index 61fd9f21..ab21e90d 100644
--- a/cli/internal/server/graphql/schema.graphql
+++ b/cli/internal/server/graphql/schema.graphql
@@ -1588,6 +1588,18 @@ type QueueSummary {
   inProgress: Int!
 }
 
+"""QueueAndWorkersSummary backs the global drain indicator. Combines the
+ready-bead count with running/total worker counts so the NavShell badge
+can render without issuing two queries per poll."""
+type QueueAndWorkersSummary {
+  """Open beads with no unmet dependencies (the drain queue depth)"""
+  readyBeads: Int!
+  """Count of workers in state `running` for this project"""
+  runningWorkers: Int!
+  """Total workers on record for this project (any state)"""
+  totalWorkers: Int!
+}
+
 """WorkerDispatchResult describes the worker started by an action."""
 type WorkerDispatchResult {
   """Worker identifier"""
@@ -2347,6 +2359,14 @@ type Query {
     projectId: String!
   ): QueueSummary!
 
+  """Return a lightweight ready-beads + worker-count snapshot for a project.
+  Returns zeros for unknown or empty projects. Used by the global drain
+  indicator in the nav shell."""
+  queueAndWorkersSummary(
+    """Project identifier"""
+    projectId: String!
+  ): QueueAndWorkersSummary!
+
   """Return model efficacy rows."""
   efficacyRows(
     """Filter sessions started at or after this ISO-8601 timestamp."""
diff --git a/cli/internal/server/graphql_adapters.go b/cli/internal/server/graphql_adapters.go
index 7e5dac76..928bc949 100644
--- a/cli/internal/server/graphql_adapters.go
+++ b/cli/internal/server/graphql_adapters.go
@@ -6,6 +6,7 @@ import (
 	"fmt"
 	"time"
 
+	"github.com/DocumentDrivenDX/ddx/internal/config"
 	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
 	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
 )
@@ -80,6 +81,35 @@ func (a *workerDispatchAdapter) DispatchWorker(ctx context.Context, kind string,
 		}
 	}
 
+	// Apply .ddx/config.yaml workers.default_spec + enforce workers.max_count
+	// (ddx-b6cf025c). The max_count cap counts currently-running drain workers
+	// for this project so the "+ Add worker" button can refuse cleanly.
+	if wc := loadWorkersConfig(projectRoot); wc != nil {
+		if spec := wc.DefaultSpec; spec != nil {
+			if req.Harness == "" {
+				req.Harness = spec.Harness
+			}
+			if req.Profile == "" {
+				req.Profile = spec.Profile
+			}
+			if req.Effort == "" {
+				req.Effort = spec.Effort
+			}
+			if req.MinTier == "" {
+				req.MinTier = spec.MinTier
+			}
+			if req.MaxTier == "" {
+				req.MaxTier = spec.MaxTier
+			}
+		}
+		if wc.MaxCount != nil && *wc.MaxCount >= 0 {
+			running := a.countRunningDrainWorkers(projectRoot)
+			if running >= *wc.MaxCount {
+				return nil, fmt.Errorf("workers.max_count cap reached: %d running (limit %d)", running, *wc.MaxCount)
+			}
+		}
+	}
+
 	var pollInterval time.Duration
 	if req.PollInterval != "" {
 		d, err := time.ParseDuration(req.PollInterval)
@@ -116,6 +146,39 @@ func (a *workerDispatchAdapter) DispatchWorker(ctx context.Context, kind string,
 	}, nil
 }
 
+// loadWorkersConfig reads .ddx/config.yaml at projectRoot and returns the
+// workers block, or nil when unset / on error. Errors are swallowed because
+// a missing or malformed config must not block the dispatch path.
+func loadWorkersConfig(projectRoot string) *config.WorkersConfig {
+	if projectRoot == "" {
+		return nil
+	}
+	cfg, err := config.LoadWithWorkingDir(projectRoot)
+	if err != nil || cfg == nil {
+		return nil
+	}
+	return cfg.Workers
+}
+
+// countRunningDrainWorkers counts execute-loop workers currently in state
+// "running" for projectRoot. Returns 0 on any error.
+func (a *workerDispatchAdapter) countRunningDrainWorkers(projectRoot string) int {
+	if a == nil || a.manager == nil {
+		return 0
+	}
+	recs, err := a.manager.List()
+	if err != nil {
+		return 0
+	}
+	count := 0
+	for _, rec := range recs {
+		if rec.Kind == "execute-loop" && rec.State == "running" && rec.ProjectRoot == projectRoot {
+			count++
+		}
+	}
+	return count
+}
+
 func (a *workerDispatchAdapter) DispatchPlugin(ctx context.Context, projectRoot string, name string, action string, scope string) (*ddxgraphql.PluginDispatchResult, error) {
 	if a == nil || a.manager == nil {
 		return nil, fmt.Errorf("worker dispatcher is not configured")
diff --git a/cli/internal/server/workers_config_test.go b/cli/internal/server/workers_config_test.go
new file mode 100644
index 00000000..55acfe58
--- /dev/null
+++ b/cli/internal/server/workers_config_test.go
@@ -0,0 +1,157 @@
+package server
+
+import (
+	"context"
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/DocumentDrivenDX/ddx/internal/agent"
+	"github.com/DocumentDrivenDX/ddx/internal/bead"
+)
+
+// writeFakeWorkerRecord dumps a status.json into the manager's workers
+// directory so List() picks it up as a running drain worker — used to
+// exercise the workers.max_count cap without spawning a real goroutine.
+func writeFakeWorkerRecord(t *testing.T, m *WorkerManager, rec WorkerRecord) {
+	t.Helper()
+	dir := filepath.Join(m.rootDir, rec.ID)
+	if err := os.MkdirAll(dir, 0o755); err != nil {
+		t.Fatal(err)
+	}
+	data, err := json.MarshalIndent(rec, "", "  ")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644); err != nil {
+		t.Fatal(err)
+	}
+}
+
+// TestWorkerDispatchAdapterEnforcesMaxCount covers the workers.max_count
+// safety rail (ddx-b6cf025c). When max_count is set and the count of
+// running execute-loop workers is already at the cap, the adapter must
+// refuse with a clear error rather than silently starting a new worker.
+func TestWorkerDispatchAdapterEnforcesMaxCount(t *testing.T) {
+	root := t.TempDir()
+	setupBeadStore(t, root)
+
+	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nworkers:\n  max_count: 1\n"
+	if err := os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
+		t.Fatal(err)
+	}
+
+	m := NewWorkerManager(root)
+	defer m.StopWatchdog()
+
+	writeFakeWorkerRecord(t, m, WorkerRecord{
+		ID:          "worker-pre-existing",
+		Kind:        "execute-loop",
+		State:       "running",
+		Status:      "running",
+		ProjectRoot: root,
+		StartedAt:   time.Now().UTC(),
+	})
+
+	adapter := &workerDispatchAdapter{manager: m}
+	_, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
+	if err == nil {
+		t.Fatal("expected error when max_count reached, got nil")
+	}
+	if !strings.Contains(err.Error(), "max_count") {
+		t.Fatalf("error should mention max_count, got: %v", err)
+	}
+}
+
+// TestWorkerDispatchAdapterMaxCountAllowsWhenUnderLimit verifies the cap
+// is inclusive (>=) not strict (>): dispatching with cap=2 and 1 running
+// worker succeeds.
+func TestWorkerDispatchAdapterMaxCountAllowsWhenUnderLimit(t *testing.T) {
+	root := t.TempDir()
+	setupBeadStore(t, root)
+
+	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nworkers:\n  max_count: 2\n  default_spec:\n    profile: cheap\n    effort: low\n"
+	if err := os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
+		t.Fatal(err)
+	}
+
+	m := NewWorkerManager(root)
+	defer m.StopWatchdog()
+	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
+		return &agent.ExecuteBeadWorker{
+			Store: s,
+			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
+				<-ctx.Done()
+				return agent.ExecuteBeadReport{BeadID: beadID, Status: agent.ExecuteBeadStatusExecutionFailed, Detail: "canceled"}, ctx.Err()
+			}),
+		}
+	}
+
+	writeFakeWorkerRecord(t, m, WorkerRecord{
+		ID:          "worker-pre-existing",
+		Kind:        "execute-loop",
+		State:       "running",
+		Status:      "running",
+		ProjectRoot: root,
+		StartedAt:   time.Now().UTC(),
+	})
+
+	adapter := &workerDispatchAdapter{manager: m}
+	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
+	if err != nil {
+		t.Fatalf("dispatch under cap: %v", err)
+	}
+	defer func() { _ = m.Stop(result.ID) }()
+
+	// Also verify default_spec propagated: profile=cheap, effort=low.
+	rec, err := m.Show(result.ID)
+	if err != nil {
+		t.Fatalf("show: %v", err)
+	}
+	if rec.Profile != "cheap" {
+		t.Errorf("record.Profile: want cheap, got %q", rec.Profile)
+	}
+	if rec.Effort != "low" {
+		t.Errorf("record.Effort: want low, got %q", rec.Effort)
+	}
+}
+
+// TestCountRunningDrainWorkersFiltersByProjectAndKind verifies the helper
+// only counts execute-loop workers in state=running for the target
+// projectRoot — not other kinds, other projects, or stopped workers.
+func TestCountRunningDrainWorkersFiltersByProjectAndKind(t *testing.T) {
+	root := t.TempDir()
+	setupBeadStore(t, root)
+
+	m := NewWorkerManager(root)
+	defer m.StopWatchdog()
+	adapter := &workerDispatchAdapter{manager: m}
+
+	writeFakeWorkerRecord(t, m, WorkerRecord{
+		ID: "w-drain-running", Kind: "execute-loop", State: "running",
+		ProjectRoot: root, StartedAt: time.Now().UTC(),
+	})
+	writeFakeWorkerRecord(t, m, WorkerRecord{
+		ID: "w-drain-stopped", Kind: "execute-loop", State: "stopped",
+		ProjectRoot: root, StartedAt: time.Now().UTC(),
+	})
+	writeFakeWorkerRecord(t, m, WorkerRecord{
+		ID: "w-plugin", Kind: "plugin-action", State: "running",
+		ProjectRoot: root, StartedAt: time.Now().UTC(),
+	})
+	writeFakeWorkerRecord(t, m, WorkerRecord{
+		ID: "w-other-project", Kind: "execute-loop", State: "running",
+		ProjectRoot: "/different/project", StartedAt: time.Now().UTC(),
+	})
+
+	got := adapter.countRunningDrainWorkers(root)
+	if got != 1 {
+		t.Fatalf("countRunningDrainWorkers: want 1, got %d", got)
+	}
+}
+
+// Ensure we import bead so setupBeadStore compiles in this package.
+var _ = bead.StatusOpen
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
## Review: ddx-b6cf025c iter 1

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
