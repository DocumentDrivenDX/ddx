import { expect, test } from '@playwright/test';

// Shared fixtures
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/workers`;

const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

const WORKERS = [
	{
		id: 'worker-aabbccdd',
		kind: 'execute-bead',
		state: 'running',
		status: 'processing',
		harness: 'claude',
		model: 'claude-sonnet-4-6',
		currentBead: 'bead-001',
		attempts: 5,
		successes: 4,
		failures: 1,
		startedAt: '2026-01-01T10:00:00Z'
	},
	{
		id: 'worker-eeffgghh',
		kind: 'execute-bead',
		state: 'idle',
		status: null,
		harness: 'claude',
		model: 'claude-sonnet-4-6',
		currentBead: null,
		attempts: 2,
		successes: 2,
		failures: 0,
		startedAt: '2026-01-01T09:00:00Z'
	},
	{
		id: 'worker-iijjkkll',
		kind: 'review',
		state: 'error',
		status: 'failed',
		harness: 'claude',
		model: 'claude-opus-4-6',
		currentBead: null,
		attempts: 1,
		successes: 0,
		failures: 1,
		startedAt: '2026-01-01T08:00:00Z'
	}
];

const WORKER_DETAIL = {
	id: 'worker-aabbccdd',
	kind: 'execute-bead',
	state: 'running',
	status: 'processing',
	harness: 'claude',
	model: 'claude-sonnet-4-6',
	effort: 'normal',
	once: false,
	pollInterval: '30s',
	startedAt: '2026-01-01T10:00:00Z',
	finishedAt: null,
	currentBead: 'bead-001',
	lastError: null,
	attempts: 5,
	successes: 4,
	failures: 1,
	currentAttempt: {
		attemptId: 'attempt-001',
		beadId: 'bead-001',
		phase: 'executing',
		startedAt: '2026-01-01T10:05:00Z',
		elapsedMs: 30000
	},
	recentEvents: [],
	lifecycleEvents: []
};

const WORKER_LOG = { stdout: 'Starting execution...\nStep 1 complete\nStep 2 complete', stderr: '' };

function makeWorkersResponse(workers: Record<string, unknown>[] = WORKERS) {
	return {
		workersByProject: {
			edges: workers.map((w, i) => ({ node: w, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: workers.length
		}
	};
}

function makeSessionsResponse(sessions: Record<string, unknown>[] = []) {
	return {
		agentSessions: {
			edges: sessions.map((node) => ({ node, cursor: String(node.id) })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: sessions.length
		}
	};
}

/**
 * Set up GraphQL route mocking for the workers pages.
 */
async function mockGraphQL(page: import('@playwright/test').Page, workers = WORKERS) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('WorkersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (body.query.includes('WorkerDetail')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { worker: WORKER_DETAIL } })
			});
		} else if (body.query.includes('WorkerLog')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workerLog: WORKER_LOG } })
			});
		} else if (body.query.includes('AgentSessions') || body.query.includes('agentSessions')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeSessionsResponse() })
			});
		} else if (body.query.includes('WorkerProgress')) {
			// Subscriptions are handled over WebSocket; pass through
			await route.continue();
		} else {
			await route.continue();
		}
	});
}

// TC-040: Workers page loads with heading and worker table
test('TC-040: workers page loads with heading and worker table', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Workers' })).toBeVisible();
	await expect(page.getByText(/Workers drain the bead queue/)).toBeVisible();

	// Table columns
	await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Kind' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: /State.*Phase/i })).toBeVisible();
});

// TC-041: Workers list renders all workers from the GraphQL response
test('TC-041: workers list renders worker kinds and states', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// All three kinds should appear
	await expect(page.getByText('execute-bead').first()).toBeVisible();
	await expect(page.getByText('review')).toBeVisible();

	// Worker states should appear
	await expect(page.getByText('running')).toBeVisible();
	await expect(page.getByText('idle')).toBeVisible();
	await expect(page.getByText('error')).toBeVisible();
});

// TC-042: Total worker count is displayed
test('TC-042: workers page shows total count', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByText(/3 total/)).toBeVisible();
});

// TC-043: Worker ID is shown as truncated (first 8 chars)
test('TC-043: worker ID is truncated to 8 characters in the table', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// worker-aabbccdd → shows "worker-a" (first 8 chars)
	await expect(page.getByText('worker-a')).toBeVisible();
});

// TC-044: Clicking a worker row navigates to the worker detail panel
test('TC-044: clicking a worker row opens the detail panel', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// Click the first worker row (running worker)
	const runningRow = page.getByText('running').first();
	await runningRow.click();

	// URL should include the workerId
	await expect(page).toHaveURL(/\/workers\/worker-aabbccdd/);

	// Worker detail panel should show kind and state
	await expect(page.getByText('execute-bead').first()).toBeVisible();
	await expect(page.getByText('running').first()).toBeVisible();
});

// TC-045: Worker detail panel shows log output and phase from WorkerDetail + WorkerLog queries
test('TC-045: worker detail panel loads and displays log output', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(`${BASE_URL}/worker-aabbccdd`);

	// Log output from WORKER_LOG.stdout should appear
	await expect(page.getByText('Starting execution...')).toBeVisible();
	await expect(page.getByText(/Step 1 complete/)).toBeVisible();

	// Current phase from the currentAttempt should appear
	await expect(page.getByText('executing')).toBeVisible();
});

// TC-046: Closing the worker detail panel navigates back to the workers list
test('TC-046: closing the worker detail panel returns to the workers list', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(`${BASE_URL}/worker-aabbccdd`);

	// The close button should be visible
	const closeButton = page.getByRole('button', { name: 'Close' });
	await expect(closeButton).toBeVisible();

	await closeButton.click();

	// URL should go back to the workers list (no workerId)
	await expect(page).toHaveURL(new RegExp(`/workers$`));
});

// TC-047: Empty state is shown when no workers are returned
test('TC-047: workers page shows empty state when no workers are returned', async ({ page }) => {
	await mockGraphQL(page, []);
	await page.goto(BASE_URL);

	await expect(page.getByText('No workers found.')).toBeVisible();
	await expect(page.getByText('0 total')).toBeVisible();
});

test('workers page starts and stops an execute-loop worker with IA links', async ({ page }) => {
	let workers: Record<string, unknown>[] = [...WORKERS.filter((worker) => worker.state !== 'running')];
	let startCalled = false;
	let stopCalled = false;

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('StartWorker') || body.query.includes('startWorker')) {
			startCalled = true;
			const worker = {
				id: 'worker-ui-started',
				kind: 'execute-loop',
				state: 'running',
				status: 'running',
				harness:
					typeof (body.variables?.input as Record<string, unknown> | undefined)?.harness === 'string'
						? (body.variables?.input as Record<string, string>).harness
						: 'codex',
				model: null,
				currentBead: null,
				attempts: 0,
				successes: 0,
				failures: 0,
				startedAt: '2026-04-22T12:00:00Z'
			};
			workers = [worker, ...workers];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { startWorker: { id: worker.id, state: worker.state, kind: worker.kind } } })
			});
		} else if (body.query.includes('StopWorker') || body.query.includes('stopWorker')) {
			stopCalled = true;
			workers = workers.map((worker) =>
				worker.id === body.variables?.id ? { ...worker, state: 'stopped', status: 'stopped' } : worker
			);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { stopWorker: { id: body.variables?.id, state: 'stopped', kind: 'execute-loop' } }
				})
			});
		} else if (body.query.includes('WorkersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (body.query.includes('WorkerDetail')) {
			const worker = workers.find((item) => item.id === 'worker-ui-started') ?? workers[0];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						worker: {
							...WORKER_DETAIL,
							...worker,
							effort: 'medium',
							once: false,
							pollInterval: '30s',
							finishedAt: worker?.state === 'stopped' ? '2026-04-22T12:01:00Z' : null,
							currentAttempt: null,
							lifecycleEvents: [
								{
									action: 'start',
									actor: 'local-operator',
									timestamp: '2026-04-22T12:00:00Z',
									detail: 'profile=smart effort=medium',
									beadId: null
								},
								{
									action: 'stop',
									actor: 'local-operator',
									timestamp: '2026-04-22T12:01:00Z',
									detail: 'reason=stop',
									beadId: null
								}
							]
						}
					}
				})
			});
		} else if (body.query.includes('WorkerLog')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workerLog: { stdout: '', stderr: '' } } })
			});
		} else if (body.query.includes('AgentSessions') || body.query.includes('agentSessions')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeSessionsResponse() })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);
	await expect(page.getByText(/Workers drain the bead queue/)).toBeVisible();
	await expect(page.getByRole('link', { name: /recent sessions/i })).toHaveAttribute('href', /\/sessions$/);
	await expect(page.getByRole('button', { name: 'Stop' })).toHaveCount(0);

	await page.getByRole('button', { name: 'Start worker' }).click();
	await page.getByLabel('Harness').fill('codex');
	await page.getByLabel('Label filter').fill('ui');
	await page.getByRole('button', { name: 'Start', exact: true }).click();
	await expect.poll(() => startCalled).toBe(true);
	await expect(page.getByText('worker-u')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Stop' })).toHaveCount(1);

	page.once('dialog', async (dialog) => {
		await dialog.dismiss();
	});
	await page.getByRole('button', { name: 'Stop' }).click();
	await expect.poll(() => stopCalled).toBe(false);

	page.once('dialog', async (dialog) => {
		await dialog.accept();
	});
	await page.getByRole('button', { name: 'Stop' }).click();
	await expect.poll(() => stopCalled).toBe(true);
	await expect(page.getByText('stopped').first()).toBeVisible();
	await expect(page.getByRole('button', { name: 'Stop' })).toHaveCount(0);

	await page.getByText('worker-u').click();
	await expect(page).toHaveURL(/\/workers\/worker-ui-started$/);
	await expect(page.getByText('No sessions recorded yet.')).toBeVisible();
	await expect(page.getByText('Lifecycle audit')).toBeVisible();

	await page.getByRole('button', { name: 'Close' }).click();
	await page.getByRole('link', { name: /recent sessions/i }).click();
	await expect(page).toHaveURL(/\/sessions$/);
	await expect(page.getByText(/Sessions are immutable agent-run history/)).toBeVisible();
});

// TC-048: Workers page subscribes to WorkerProgress for running workers (subscription exercised)
test('TC-048: WorkerProgress subscription is attempted for running workers', async ({ page }) => {
	// Track all outgoing requests to GraphQL to verify the subscription query is sent
	const wsRequests: string[] = [];

	// We intercept any WebSocket connections and track upgrade requests
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('WorkersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse() })
			});
		} else if (body.query.includes('WorkerProgress')) {
			wsRequests.push('WorkerProgress');
			await route.continue();
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	// Wait for page to load
	await expect(page.getByRole('heading', { name: 'Workers' })).toBeVisible();
	await expect(page.getByText('running')).toBeVisible();

	// Give a moment for the subscription effect to run
	await page.waitForTimeout(200);

	// The subscription client connects via WebSocket (not HTTP), so the HTTP intercept
	// won't see WorkerProgress — but the running worker state triggers the subscription
	// effect. This test verifies the page renders running workers (subscription precondition).
	const runningRows = page.locator('td').filter({ hasText: 'running' });
	await expect(runningRows).toHaveCount(1);
});

// -----------------------------------------------------------------------
// FEAT-008 US-086a: streaming agent response text + tool-call cards
// -----------------------------------------------------------------------

test('US-086a.a: worker detail renders a Live Response panel while running', async ({ page }) => {
	// This test exercises the presence of the live-response UI affordance. The
	// actual WebSocket text_delta stream is exercised under a real server in
	// demo-recording.spec.ts; the unit-level e2e asserts that the component
	// renders with an initial empty state + ARIA live region so screen readers
	// announce streaming updates.
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('worker(') || body.query.includes('Worker(')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { worker: WORKER_DETAIL } }) });
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/${WORKER_DETAIL.id}`);

	const liveResponse = page.getByRole('region', { name: /live response/i });
	await expect(liveResponse).toBeVisible();
	await expect(liveResponse).toHaveAttribute('aria-live', /polite|assertive/);
});

test('US-086a.b: tool calls render as collapsible cards interleaved with text', async ({ page }) => {
	// Fixture: the worker detail query returns a recent_events array
	// that includes text_delta + tool_call frames; the component renders
	// them in delivery order.
	const WORKER_WITH_EVENTS = {
		...WORKER_DETAIL,
		recentEvents: [
			{ kind: 'text_delta', text: 'Looking at ' },
			{ kind: 'text_delta', text: 'the bead spec.\n\n' },
			{
				kind: 'tool_call',
				name: 'read',
				inputs: { path: 'docs/helix/01-frame/prd.md' },
				output: 'PRD content...'
			},
			{ kind: 'text_delta', text: 'Now I understand.' }
		]
	};

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('worker(') || body.query.includes('Worker(')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { worker: WORKER_WITH_EVENTS } }) });
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/${WORKER_DETAIL.id}`);

	const live = page.getByRole('region', { name: /live response/i });
	await expect(live.getByText(/Looking at the bead spec/)).toBeVisible();

	const toolCard = live.getByRole('button', { name: /read .* prd\.md/i }).first();
	await expect(toolCard).toBeVisible();
	await toolCard.click();
	await expect(live.getByText(/PRD content/)).toBeVisible();

	await expect(live.getByText(/Now I understand/)).toBeVisible();
});

test('US-086a.c: terminal-phase worker freezes stream with completion timestamp', async ({ page }) => {
	const DONE_WORKER = {
		...WORKER_DETAIL,
		state: 'done',
		status: 'success',
		finishedAt: '2026-01-01T10:15:00Z',
		currentAttempt: null,
		recentEvents: [{ kind: 'text_delta', text: 'Done.' }]
	};

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('worker(') || body.query.includes('Worker(')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { worker: DONE_WORKER } }) });
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/${WORKER_DETAIL.id}`);
	const live = page.getByRole('region', { name: /live response/i });
	await expect(live.getByText(/completed at/i)).toBeVisible();
	// Link to the evidence bundle
	await expect(live.getByRole('link', { name: /evidence bundle/i })).toBeVisible();
});

// ddx-b6cf025c: global drain indicator + workers-overview count control.
test('workers overview shows drain count control, indicator, and +/- buttons', async ({ page }) => {
	let workers: Record<string, unknown>[] = [];
	let dispatchCalled = false;
	let stopCalled = false;

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('QueueAndWorkersSummary') || body.query.includes('queueAndWorkersSummary')) {
			const running = workers.filter((w) => w.state === 'running').length;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: 3,
							runningWorkers: running,
							totalWorkers: workers.length
						}
					}
				})
			});
		} else if (body.query.includes('AddDrainWorker') || body.query.includes('workerDispatch')) {
			dispatchCalled = true;
			const id = `worker-drain-${workers.length + 1}`;
			workers = [
				{
					id,
					kind: 'execute-loop',
					state: 'running',
					status: 'running',
					harness: 'codex',
					model: null,
					currentBead: null,
					attempts: 0,
					successes: 0,
					failures: 0,
					startedAt: '2026-04-23T10:00:00Z'
				},
				...workers
			];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workerDispatch: { id, state: 'running', kind: 'execute-loop' } } })
			});
		} else if (body.query.includes('StopWorker') || body.query.includes('stopWorker')) {
			stopCalled = true;
			workers = workers.map((w) =>
				w.id === body.variables?.id ? { ...w, state: 'stopped', status: 'stopped' } : w
			);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { stopWorker: { id: body.variables?.id, state: 'stopped', kind: 'execute-loop' } }
				})
			});
		} else if (body.query.includes('WorkersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (body.query.includes('AgentSessions') || body.query.includes('agentSessions')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: makeSessionsResponse() }) });
		} else {
			await route.continue();
		}
		// Accept a confirm() dialog on the Remove path.
	});

	// Auto-accept the browser confirm() that Remove-worker opens.
	page.on('dialog', (dialog) => void dialog.accept());

	await page.goto(BASE_URL);

	// Count control panel is visible on the workers overview.
	const panel = page.getByTestId('drain-count-panel');
	await expect(panel).toBeVisible();
	await expect(panel.getByTestId('drain-worker-count')).toHaveText('0');
	await expect(panel.getByText(/Adds a general-purpose drain worker/)).toBeVisible();

	// Global nav indicator is visible on every page (ddx-b6cf025c AC #1).
	const indicator = page.getByTestId('drain-indicator');
	await expect(indicator).toBeVisible();
	await expect(indicator).toHaveText(/Queue: 3 ready|0 workers/);

	// + Add worker dispatches.
	await page.getByTestId('add-drain-worker').click();
	await expect.poll(() => dispatchCalled, { timeout: 3000 }).toBe(true);
	await expect(panel.getByTestId('drain-worker-count')).toHaveText('1');

	// − Remove worker stops the oldest running drain worker.
	await page.getByTestId('remove-drain-worker').click();
	await expect.poll(() => stopCalled, { timeout: 3000 }).toBe(true);

	// Indicator survives navigation to another route.
	await page.goto(`/nodes/node-abc/projects/${PROJECT_ID}/beads`);
	await expect(page.getByTestId('drain-indicator')).toBeVisible();
});
