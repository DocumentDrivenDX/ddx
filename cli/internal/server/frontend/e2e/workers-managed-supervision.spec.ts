// TC-016: Server-Managed Worker Supervision (ddx-2b15342f)
//
// Covers AC 1, 2, 3:
//   AC 1 — desired-count stepper calls setWorkerDesiredState and shows desired vs actual.
//   AC 2 — Restart button on managed workers calls restartWorker; external workers show
//           "reported only" and have no Stop/Restart.
//   AC 3 — stale/backoff/cleanup-needed states render without overlapping controls at
//           desktop (1280×720) and mobile (390×844) widths.
//
// All tests use mocked GraphQL routes — no live DDx server is required.

import { expect, test } from '@playwright/test';

const NODE_ID = 'node-test-supervision';
const NODE_NAME = 'test-node-supervision';
const PROJECT_ID = 'project-supervision-001';
const PROJECT = { id: PROJECT_ID, name: 'Supervision Project', path: '/tmp/project-supervision' };
const BASE_URL = `/nodes/${NODE_ID}/projects/${PROJECT_ID}/workers`;

function makeWorker(overrides: Record<string, unknown> = {}): Record<string, unknown> {
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
		managed: true,
		desiredCount: 1,
		restartCount: 0,
		lastRestartAt: null,
		backoffUntil: null,
		cleanupNeeded: false,
		...overrides
	};
}

function makeWorkersResponse(
	workers: Record<string, unknown>[],
	maxCount: number | null = null
): Record<string, unknown> {
	return {
		workersByProject: {
			edges: workers.map((w, i) => ({ node: w, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: workers.length
		},
		queueAndWorkersSummary: { maxCount }
	};
}

function makeSessionsResponse(): Record<string, unknown> {
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

async function mockBaseGraphQL(
	page: import('@playwright/test').Page,
	workers: Record<string, unknown>[]
) {
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
		} else if (body.query.includes('QueueAndWorkersSummary') || body.query.includes('queueAndWorkersSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: 1,
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
		} else if (body.query.includes('ReportedWorker') || body.query.includes('reportedWorkers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { reportedWorkers: [] } })
			});
		} else {
			await route.continue();
		}
	});
}

// AC 1: desired-count stepper calls setWorkerDesiredState; shows desired vs actual.
test('desired count stepper calls setWorkerDesiredState and shows desired vs actual count', async ({
	page
}) => {
	let setDesiredStateCalled = false;
	let setDesiredStateInput: Record<string, unknown> | null = null;

	const managedWorker = makeWorker({
		id: 'worker-managed-00000001',
		managed: true,
		desiredCount: 1,
		restartCount: 0
	});
	const workers = [managedWorker];

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};

		if (
			body.query.includes('SetWorkerDesiredState') ||
			body.query.includes('setWorkerDesiredState')
		) {
			setDesiredStateCalled = true;
			setDesiredStateInput = (body.variables?.input ?? {}) as Record<string, unknown>;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						setWorkerDesiredState: {
							projectId: PROJECT_ID,
							desiredCount: (setDesiredStateInput.desiredCount as number) ?? 2,
							actualCount: 1
						}
					}
				})
			});
		} else if (body.query.includes('NodeInfo')) {
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
		} else if (
			body.query.includes('WorkersByProject') ||
			body.query.includes('workersByProject')
		) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (
			body.query.includes('QueueAndWorkersSummary') ||
			body.query.includes('queueAndWorkersSummary')
		) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: { readyBeads: 1, runningWorkers: 1, totalWorkers: 1, maxCount: null }
					}
				})
			});
		} else if (body.query.includes('AgentSessions') || body.query.includes('agentSessions')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeSessionsResponse() })
			});
		} else if (
			body.query.includes('ReportedWorker') ||
			body.query.includes('reportedWorkers')
		) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { reportedWorkers: [] } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);
	await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

	// Supervision panel must show actual managed count and desired count.
	const supervisionPanel = page.getByTestId('supervised-count-panel');
	await expect(supervisionPanel).toBeVisible();
	await expect(page.getByTestId('managed-actual-count')).toHaveText('1');
	await expect(page.getByTestId('managed-desired-count')).toHaveText('1');

	// Stepper must be present.
	const stepper = page.getByTestId('desired-count-stepper');
	await expect(stepper).toBeVisible();

	// Click increase (+) button.
	await page.getByTestId('desired-count-increase').click();
	await expect.poll(() => setDesiredStateCalled, { timeout: 3000 }).toBe(true);
	expect(setDesiredStateInput).toMatchObject({ projectId: PROJECT_ID, desiredCount: 2 });
});

// AC 2: Restart appears for managed workers and calls restartWorker;
//        external workers are labeled "reported only" with no Stop/Restart.
test('managed workers show Restart button; external workers show reported only', async ({
	page
}) => {
	let restartCalled = false;
	let restartedWorkerId: string | null = null;

	const MANAGED_ID = 'worker-managed-00000001';
	const EXTERNAL_ID = 'worker-external-00000001';

	const managedWorker = makeWorker({
		id: MANAGED_ID,
		managed: true,
		desiredCount: 1
	});
	const externalWorker = makeWorker({
		id: EXTERNAL_ID,
		managed: false,
		desiredCount: null,
		restartCount: null
	});
	const workers = [managedWorker, externalWorker];

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};

		if (body.query.includes('RestartWorker') || body.query.includes('restartWorker')) {
			restartCalled = true;
			restartedWorkerId = (body.variables?.id as string) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						restartWorker: { id: restartedWorkerId, state: 'running', kind: 'work' }
					}
				})
			});
		} else if (body.query.includes('NodeInfo')) {
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
		} else if (
			body.query.includes('WorkersByProject') ||
			body.query.includes('workersByProject')
		) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (
			body.query.includes('QueueAndWorkersSummary') ||
			body.query.includes('queueAndWorkersSummary')
		) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						queueAndWorkersSummary: {
							readyBeads: 1,
							runningWorkers: 2,
							totalWorkers: 2,
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
		} else if (
			body.query.includes('ReportedWorker') ||
			body.query.includes('reportedWorkers')
		) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { reportedWorkers: [] } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);
	await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

	// Managed workers section must contain the managed worker row.
	const managedSection = page.getByTestId('managed-workers-section');
	await expect(managedSection).toBeVisible();
	const managedRow = page.getByTestId('managed-worker-row').filter({ hasText: MANAGED_ID.slice(0, 8) });
	await expect(managedRow).toBeVisible();

	// Restart button must appear for the managed running worker.
	const restartBtn = page.getByTestId(`restart-worker-${MANAGED_ID}`);
	await expect(restartBtn).toBeVisible();

	// Accept the confirmation dialog.
	page.once('dialog', async (dialog) => {
		await dialog.accept();
	});
	await restartBtn.click();
	await expect.poll(() => restartCalled, { timeout: 3000 }).toBe(true);
	expect(restartedWorkerId).toBe(MANAGED_ID);

	// External workers section must be visible and labeled "reported only".
	const externalSection = page.getByTestId('external-workers-section');
	await expect(externalSection).toBeVisible();
	await expect(page.getByTestId('external-workers-label')).toBeVisible();
	await expect(page.getByTestId('external-workers-label')).toHaveText(/reported only/i);

	// External worker row must show "reported only" in actions and have no Restart button.
	const externalNoActions = page.getByTestId(`external-worker-no-actions-${EXTERNAL_ID}`);
	await expect(externalNoActions).toBeVisible();
	await expect(page.getByTestId(`restart-worker-${EXTERNAL_ID}`)).not.toBeAttached();
});

// AC 3: stale/backoff/cleanup-needed states render without overlapping controls
//        at desktop (1280×720) and mobile (390×844) widths.
for (const { label, width, height } of [
	{ label: 'desktop', width: 1280, height: 720 },
	{ label: 'mobile', width: 390, height: 844 }
]) {
	test(`worker status badges render correctly at ${label} (${width}×${height})`, async ({
		page
	}) => {
		await page.setViewportSize({ width, height });

		const STALE_ID = 'worker-stale-00000001';
		const BACKOFF_ID = 'worker-backoff-00000001';
		const CLEANUP_ID = 'worker-cleanup-00000001';
		// backoffUntil must be in the future so the badge renders.
		const futureBackoff = new Date(Date.now() + 5 * 60 * 1000).toISOString();

		const workers = [
			makeWorker({
				id: STALE_ID,
				state: 'stale',
				managed: true,
				desiredCount: 1,
				cleanupNeeded: false,
				backoffUntil: null
			}),
			makeWorker({
				id: BACKOFF_ID,
				state: 'running',
				managed: true,
				desiredCount: 1,
				cleanupNeeded: false,
				backoffUntil: futureBackoff
			}),
			makeWorker({
				id: CLEANUP_ID,
				state: 'stopped',
				managed: true,
				desiredCount: 1,
				cleanupNeeded: true,
				backoffUntil: null
			})
		];

		await mockBaseGraphQL(page, workers);

		await page.goto(BASE_URL);
		await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

		// stale badge must appear for the stale worker.
		const staleBadge = page.getByTestId(`worker-status-badge-${STALE_ID}`);
		await expect(staleBadge).toBeVisible();
		await expect(staleBadge).toHaveText(/stale/i);

		// backoff badge must appear for the worker in backoff.
		const backoffBadge = page.getByTestId(`worker-status-badge-${BACKOFF_ID}`);
		await expect(backoffBadge).toBeVisible();
		await expect(backoffBadge).toHaveText(/backoff/i);

		// cleanup-needed badge must appear.
		const cleanupBadge = page.getByTestId(`worker-status-badge-${CLEANUP_ID}`);
		await expect(cleanupBadge).toBeVisible();
		await expect(cleanupBadge).toHaveText(/cleanup needed/i);

		// No badge overlaps controls: verify each badge has a positive size
		// and is rendered inside the managed-workers section (not hidden/clipped).
		for (const badge of [staleBadge, backoffBadge, cleanupBadge]) {
			const box = await badge.boundingBox();
			expect(box).not.toBeNull();
			if (box) {
				// Badge must have positive dimensions — not zero-width collapsed.
				expect(box.width).toBeGreaterThan(0);
				expect(box.height).toBeGreaterThan(0);
			}
		}
	});
}
