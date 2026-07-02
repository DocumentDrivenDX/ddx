// TC-016: Worker Single Pane and Server-Managed Work
//
// workers-dispatch.spec.ts covers TC-016.3, TC-016.4, and TC-016.5 from
// TP-002 (starts two workers, refuses beyond max_count, stop with lifecycle
// event). All tests use static fixture data and mocked GraphQL routes — no
// live developer DDx state is required. Tests pass once the implementation
// beads for server-managed dispatch land.

import { expect, test } from '@playwright/test';

// Static fixture identifiers — no live server request is made.
const NODE_ID = 'node-test-dispatch';
const NODE_NAME = 'test-node-dispatch';
const PROJECT_ID = 'project-dispatch-001';
const PROJECT = { id: PROJECT_ID, name: 'Dispatch Project', path: '/tmp/project-dispatch' };

// Base URL for the project workers page where dispatch controls live.
const BASE_URL = `/nodes/${NODE_ID}/projects/${PROJECT_ID}/workers`;

function makeBaseWorker(overrides: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		id: 'worker-placeholder',
		kind: 'work',
		state: 'running',
		status: 'running',
		harness: 'claude',
		model: null,
		currentBead: null,
		attempts: 0,
		successes: 0,
		failures: 0,
		startedAt: '2026-06-01T10:00:00Z',
		...overrides
	};
}

function makeWorkersResponse(
	workers: Record<string, unknown>[],
	maxCount: number | null = null
) {
	return {
		workersByProject: {
			edges: workers.map((w, i) => ({ node: w, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: workers.length
		},
		queueAndWorkersSummary: { maxCount }
	};
}

function makeSessionsResponse() {
	return {
		agentSessions: {
			edges: [],
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: 0
		},
		sessionsCostSummary: {
			cashUsd: 0,
			subscriptionEquivUsd: 0,
			localSessionCount: 0,
			localEstimatedUsd: 0
		}
	};
}

// TC-016.3 — starts two server-managed work workers and both rows appear.
test('starts two server managed work workers', async ({ page }) => {
	let workers: Record<string, unknown>[] = [];
	let startCallCount = 0;

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: { id: NODE_ID, name: NODE_NAME } } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: [PROJECT].map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('WorkersByProject') || body.query.includes('workersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (body.query.includes('StartWorker') || body.query.includes('startWorker')) {
			startCallCount++;
			const newWorker = makeBaseWorker({
				id: `worker-dispatch-${startCallCount.toString().padStart(8, '0')}`,
				kind: 'work',
				state: 'running',
				status: 'running'
			});
			workers = [newWorker, ...workers];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						startWorker: {
							id: newWorker.id,
							state: newWorker.state,
							kind: newWorker.kind
						}
					}
				})
			});
		} else if (body.query.includes('QueueAndWorkersSummary') || body.query.includes('queueAndWorkersSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: 5,
							runningWorkers: workers.filter((w) => w.state === 'running').length,
							totalWorkers: workers.length,
							maxCount: null
						}
					}
				})
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
	await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

	// Initial state: no workers.
	await expect(page.getByText('0 total')).toBeVisible();

	// Start first worker.
	await page.getByRole('button', { name: 'Start worker' }).click();
	await page.getByRole('button', { name: 'Start', exact: true }).click();
	await expect.poll(() => startCallCount, { timeout: 3000 }).toBe(1);

	// Reload or await re-query: first worker row appears.
	await expect(page.getByText('worker-d').first()).toBeVisible();
	await expect(page.getByText('1 total')).toBeVisible();

	// Start second worker.
	await page.getByRole('button', { name: 'Start worker' }).click();
	await page.getByRole('button', { name: 'Start', exact: true }).click();
	await expect.poll(() => startCallCount, { timeout: 3000 }).toBe(2);

	// Both worker rows must appear.
	await expect(page.getByText('2 total')).toBeVisible();
	await expect(page.getByText('running').first()).toBeVisible();
});

// TC-016.4 — refuses starts beyond workers.max_count.
test('refuses starts beyond workers max count', async ({ page }) => {
	// One worker already running, cap is 1.
	const cappedWorker = makeBaseWorker({ id: 'worker-capped-00000001', state: 'running' });

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: { id: NODE_ID, name: NODE_NAME } } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: [PROJECT].map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('WorkersByProject') || body.query.includes('workersByProject')) {
			// Return capped state: 1 running worker, maxCount=1.
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse([cappedWorker], 1) })
			});
		} else if (body.query.includes('QueueAndWorkersSummary') || body.query.includes('queueAndWorkersSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: 2,
							runningWorkers: 1,
							totalWorkers: 1,
							maxCount: 1
						}
					}
				})
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
	await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

	// The add-drain-worker button (+ Add worker) must be disabled at the cap.
	const addBtn = page.getByTestId('add-drain-worker');
	await expect(addBtn).toBeVisible();
	await expect(addBtn).toBeDisabled();

	// The disabled state must convey the cap reason to the operator.
	await expect(addBtn).toHaveAttribute('title', 'at workers.max_count limit');

	// The start-worker button in the dialog path also reflects the cap.
	// If the button isn't present (only the count-panel button is shown),
	// verifying the add button disabled + title is sufficient coverage for the cap AC.
	await expect(page.getByText(/1 total/)).toBeVisible();
});

// TC-016.5 — stops a running worker and shows the lifecycle event in the detail panel.
test('stops a worker and shows the lifecycle event', async ({ page }) => {
	let workerState = 'running';
	let stopCalled = false;

	const LIFECYCLE_EVENTS_AFTER_STOP = [
		{
			action: 'start',
			actor: 'local-operator',
			timestamp: '2026-06-01T10:00:00Z',
			detail: 'profile=smart effort=medium',
			beadId: null
		},
		{
			action: 'stop',
			actor: 'local-operator',
			timestamp: '2026-06-01T10:05:00Z',
			detail: 'reason=operator-stop',
			beadId: null
		}
	];

	const WORKER_ID = 'worker-lifecycle-00000001';

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: { id: NODE_ID, name: NODE_NAME } } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: [PROJECT].map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('WorkersByProject') || body.query.includes('workersByProject')) {
			const w = makeBaseWorker({ id: WORKER_ID, state: workerState, status: workerState });
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse([w]) })
			});
		} else if (body.query.includes('StopWorker') || body.query.includes('stopWorker')) {
			stopCalled = true;
			workerState = 'stopped';
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						stopWorker: { id: WORKER_ID, state: 'stopped', kind: 'work' }
					}
				})
			});
		} else if (body.query.includes('WorkerDetail') || body.query.includes('worker(') || body.query.includes('Worker(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						worker: {
							id: WORKER_ID,
							kind: 'work',
							state: workerState,
							status: workerState,
							harness: 'claude',
							model: null,
							effort: 'medium',
							once: false,
							pollInterval: '30s',
							startedAt: '2026-06-01T10:00:00Z',
							finishedAt: workerState === 'stopped' ? '2026-06-01T10:05:00Z' : null,
							currentBead: null,
							lastError: null,
							attempts: 0,
							successes: 0,
							failures: 0,
							currentAttempt: null,
							recentEvents: [],
							lifecycleEvents: stopCalled ? LIFECYCLE_EVENTS_AFTER_STOP : [LIFECYCLE_EVENTS_AFTER_STOP[0]]
						}
					}
				})
			});
		} else if (body.query.includes('WorkerLog') || body.query.includes('workerLog')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workerLog: { stdout: '', stderr: '' } } })
			});
		} else if (body.query.includes('QueueAndWorkersSummary') || body.query.includes('queueAndWorkersSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: 1,
							runningWorkers: workerState === 'running' ? 1 : 0,
							totalWorkers: 1,
							maxCount: null
						}
					}
				})
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
	await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

	// Running worker row must be present.
	await expect(page.getByText('running')).toBeVisible();

	// Navigate to the worker detail panel.
	await page.goto(`${BASE_URL}/${WORKER_ID}`);

	// Stop button must be visible for a running worker.
	const stopBtn = page.getByRole('button', { name: 'Stop' });
	await expect(stopBtn).toBeVisible();

	// Accept the confirmation dialog that Stop opens.
	page.once('dialog', async (dialog) => {
		await dialog.accept();
	});
	await stopBtn.click();
	await expect.poll(() => stopCalled, { timeout: 3000 }).toBe(true);

	// Worker must reach a terminal state (stopped).
	await expect(page.getByText('stopped').first()).toBeVisible({ timeout: 5000 });

	// Lifecycle audit section must show the stop event.
	await expect(page.getByText('Lifecycle audit')).toBeVisible();
	await expect(page.getByText('stop')).toBeVisible();
	await expect(page.getByText('local-operator')).toBeVisible();
});
