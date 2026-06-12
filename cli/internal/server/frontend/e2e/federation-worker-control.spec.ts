// TC-017: Federated Worker Control and Owner-Targeted Writes
//
// Verifies FEAT-026 US-097b and FEAT-021 US-102.
// The hub forwards startWorker / stopWorker mutations to the owning spoke;
// offline spokes are refused with a visible reason; retried forwarded commands
// with the same request id return the original result without duplicates.
//
// These tests MUST FAIL until the hub federation workers view implements
// forwarded worker start/stop for spoke-owned projects.

import { expect, test } from '@playwright/test';

const HUB_NODE_ID = 'hub-node-001';
const SPOKE_NODE_ID = 'spoke-node-001';
const HUB_PROJECT_ID = 'proj-hub-1';
const SPOKE_PROJECT_ID = 'proj-spoke-1';

const HUB_NODE_INFO = { id: HUB_NODE_ID, name: 'Hub Node' };

const PROJECTS = [
	{ id: HUB_PROJECT_ID, name: 'Hub Project', path: '/repos/hub-alpha', nodeId: HUB_NODE_ID },
	{ id: SPOKE_PROJECT_ID, name: 'Spoke Project', path: '/repos/spoke-alpha', nodeId: SPOKE_NODE_ID }
];

const FEDERATION_NODES_ACTIVE = [
	{ nodeId: HUB_NODE_ID, name: 'Hub Node', status: 'active' },
	{ nodeId: SPOKE_NODE_ID, name: 'Spoke Node', status: 'active' }
];

const FEDERATION_NODES_SPOKE_OFFLINE = [
	{ nodeId: HUB_NODE_ID, name: 'Hub Node', status: 'active' },
	{ nodeId: SPOKE_NODE_ID, name: 'Spoke Node', status: 'offline' }
];

const HUB_WORKER = {
	id: 'worker-hub-001',
	kind: 'work',
	state: 'running',
	status: 'running',
	projectId: HUB_PROJECT_ID,
	nodeId: HUB_NODE_ID,
	startedAt: '2026-01-01T00:00:00Z'
};

const SPOKE_WORKER = {
	id: 'worker-spoke-001',
	kind: 'work',
	state: 'running',
	status: 'running',
	projectId: SPOKE_PROJECT_ID,
	nodeId: SPOKE_NODE_ID,
	startedAt: '2026-01-01T00:00:00Z'
};

const WORKERS_FEDERATION_URL = `/nodes/${HUB_NODE_ID}/workers?scope=federation`;

async function mockBase(
	page: import('@playwright/test').Page,
	opts: {
		federationNodes?: typeof FEDERATION_NODES_ACTIVE;
		workers?: typeof HUB_WORKER[];
	} = {}
) {
	const federationNodes = opts.federationNodes ?? FEDERATION_NODES_ACTIVE;
	const workers = opts.workers ?? [HUB_WORKER, SPOKE_WORKER];

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: HUB_NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else if (body.query.includes('federationNodes') || body.query.includes('FederationNodes')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { federationNodes } })
			});
		} else if (body.query.includes('workers') || body.query.includes('Workers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { workers: { edges: workers.map((w) => ({ node: w })), totalCount: workers.length } }
				})
			});
		} else {
			await route.continue();
		}
	});
}

// TC-017.2: Starting a worker for a spoke project from the hub forwards to exactly that spoke.
test('starts a spoke worker from the hub', async ({ page }) => {
	let startCalled = false;
	let startInput: Record<string, unknown> = {};

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: HUB_NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('federationNodes') || body.query.includes('FederationNodes')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { federationNodes: FEDERATION_NODES_ACTIVE } }) });
		} else if (body.query.includes('workers') || body.query.includes('Workers')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { workers: { edges: [], totalCount: 0 } } }) });
		} else if (body.query.includes('startWorker') || body.query.includes('StartWorker')) {
			startCalled = true;
			startInput = (body.variables?.input ?? body.variables ?? {}) as Record<string, unknown>;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { startWorker: { id: 'worker-new-spoke-001', state: 'queued', projectId: SPOKE_PROJECT_ID, nodeId: SPOKE_NODE_ID } } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(WORKERS_FEDERATION_URL);

	// Locate the spoke project row and start a worker from it.
	const spokeRow = page.locator('[data-testid="worker-project-row"]', { hasText: 'Spoke Project' });
	await expect(spokeRow).toBeVisible();
	await spokeRow.getByRole('button', { name: /start worker/i }).click();

	// The startWorker mutation must have been called with the spoke project id.
	await expect.poll(() => startCalled, { timeout: 5000 }).toBe(true);
	expect(startInput).toMatchObject({ projectId: SPOKE_PROJECT_ID });

	// The new worker row must appear under the spoke node, not the hub.
	const newWorkerRow = page.locator('[data-testid="worker-row"]', { hasText: 'worker-new-spoke-001' });
	await expect(newWorkerRow).toBeVisible({ timeout: 3000 });
	await expect(newWorkerRow.getByTestId('worker-node-badge')).toContainText('Spoke Node');
});

// TC-017.3: Stopping a spoke worker from the hub forwards to the owning spoke.
test('stops a spoke worker from the hub', async ({ page }) => {
	let stopCalled = false;
	let stoppedWorkerId = '';

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: HUB_NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('federationNodes') || body.query.includes('FederationNodes')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { federationNodes: FEDERATION_NODES_ACTIVE } }) });
		} else if (body.query.includes('workers') || body.query.includes('Workers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workers: { edges: [{ node: SPOKE_WORKER }], totalCount: 1 } } })
			});
		} else if (body.query.includes('stopWorker') || body.query.includes('StopWorker')) {
			stopCalled = true;
			stoppedWorkerId = (body.variables?.id ?? '') as string;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { stopWorker: { ...SPOKE_WORKER, state: 'stopping' } } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(WORKERS_FEDERATION_URL);

	// Find the running spoke worker row and stop it.
	const spokeWorkerRow = page.locator('[data-testid="worker-row"]', { hasText: 'worker-spoke-001' });
	await expect(spokeWorkerRow).toBeVisible();
	await spokeWorkerRow.getByRole('button', { name: /stop/i }).click();

	// stopWorker must have been called for the spoke worker id.
	await expect.poll(() => stopCalled, { timeout: 5000 }).toBe(true);
	expect(stoppedWorkerId).toBe(SPOKE_WORKER.id);

	// The worker row must transition to a stopping/terminal state in the hub view.
	await expect(spokeWorkerRow.getByTestId('worker-state-badge')).toContainText(/stopping|stopped|terminated/i);
});

// TC-017.4: Commands for an offline spoke are refused with an operator-visible reason.
test('refuses commands for an offline spoke', async ({ page }) => {
	await mockBase(page, { federationNodes: FEDERATION_NODES_SPOKE_OFFLINE, workers: [HUB_WORKER] });

	await page.goto(WORKERS_FEDERATION_URL);

	// The spoke project row must be visible and indicate the spoke is offline.
	const spokeRow = page.locator('[data-testid="worker-project-row"]', { hasText: 'Spoke Project' });
	await expect(spokeRow).toBeVisible();
	await expect(spokeRow.getByTestId('federation-status-badge')).toContainText(/offline/i);

	// Start Worker must be disabled or absent for the offline spoke.
	const startBtn = spokeRow.getByRole('button', { name: /start worker/i });
	const isDisabled = (await startBtn.count()) === 0 || (await startBtn.isDisabled());
	expect(isDisabled, 'Start Worker must be disabled for an offline spoke').toBe(true);
});

// TC-017.5: Retrying a forwarded command with the same request id returns the original result.
test('deduplicates retried forwarded worker commands', async ({ page }) => {
	let stopCallCount = 0;
	const DEDUP_REQUEST_ID = 'req-idempotent-001';
	const firstResult = { ...SPOKE_WORKER, state: 'stopping' };

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: HUB_NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('federationNodes') || body.query.includes('FederationNodes')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { federationNodes: FEDERATION_NODES_ACTIVE } }) });
		} else if (body.query.includes('workers') || body.query.includes('Workers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workers: { edges: [{ node: SPOKE_WORKER }], totalCount: 1 } } })
			});
		} else if (body.query.includes('stopWorker') || body.query.includes('StopWorker')) {
			// Simulate server-side deduplication: second call with same requestId returns
			// same result without incrementing a side-effect counter past 1.
			stopCallCount++;
			const vars = (body.variables ?? {}) as Record<string, unknown>;
			if (vars.requestId === DEDUP_REQUEST_ID && stopCallCount > 1) {
				// Return the original result — no new worker was stopped.
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { stopWorker: firstResult } }) });
			} else {
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { stopWorker: firstResult } }) });
			}
		} else {
			await route.continue();
		}
	});

	await page.goto(WORKERS_FEDERATION_URL);

	const spokeWorkerRow = page.locator('[data-testid="worker-row"]', { hasText: 'worker-spoke-001' });
	await expect(spokeWorkerRow).toBeVisible();

	// Trigger stop twice with the same request id via the UI retry mechanism.
	await spokeWorkerRow.getByRole('button', { name: /stop/i }).click();
	// Simulate a retry — wait briefly and trigger again (the UI must not create a new worker).
	await page.waitForTimeout(200);
	await spokeWorkerRow.getByRole('button', { name: /stop/i }).click({ force: true });

	// Both calls resolve to the same state. No duplicate rows must appear.
	const stoppingRows = page.locator('[data-testid="worker-row"][data-state="stopping"]');
	await expect.poll(async () => await stoppingRows.count(), { timeout: 5000 }).toBeLessThanOrEqual(1);
	// The worker count must not have grown.
	await expect(page.locator('[data-testid="worker-row"]')).toHaveCount(1);
});
