// TC-016: Worker Single Pane and Server-Managed Work
//
// workers-single-pane.spec.ts covers TC-016.1, TC-016.2, and the federation
// listing fixture (TC-016 federation row from US-100). All tests use static
// fixture data and mocked GraphQL routes — no live developer DDx state is
// required. These tests pass once the /nodes/:nodeId/workers route and
// node-wide workers query land (implementation beads).

import { expect, test } from '@playwright/test';

// Static fixture identifiers — no live server request is made.
const NODE_ID = 'node-test-singlePane';
const NODE_NAME = 'test-node-singlePane';

const PROJECT_ALPHA = { id: 'project-alpha', name: 'Project Alpha', path: '/tmp/project-alpha' };
const PROJECT_BETA = { id: 'project-beta', name: 'Project Beta', path: '/tmp/project-beta' };

// Workers from two different local projects, each with a project badge.
const WORKERS_MULTI_PROJECT = [
	{
		id: 'worker-aaaaaaaa',
		kind: 'execute-bead',
		state: 'running',
		status: 'processing',
		harness: 'claude',
		model: 'claude-sonnet-4-6',
		currentBead: 'bead-alpha-001',
		attempts: 3,
		successes: 2,
		failures: 1,
		startedAt: '2026-06-01T10:00:00Z',
		projectId: PROJECT_ALPHA.id,
		projectName: PROJECT_ALPHA.name
	},
	{
		id: 'worker-bbbbbbbb',
		kind: 'work',
		state: 'idle',
		status: null,
		harness: 'claude',
		model: 'claude-sonnet-4-6',
		currentBead: null,
		attempts: 5,
		successes: 5,
		failures: 0,
		startedAt: '2026-06-01T09:00:00Z',
		projectId: PROJECT_BETA.id,
		projectName: PROJECT_BETA.name
	}
];

// Worker detail fixture for the running worker.
const WORKER_DETAIL_ALPHA = {
	id: 'worker-aaaaaaaa',
	kind: 'execute-bead',
	state: 'running',
	status: 'processing',
	harness: 'claude',
	model: 'claude-sonnet-4-6',
	effort: 'normal',
	once: false,
	pollInterval: '30s',
	startedAt: '2026-06-01T10:00:00Z',
	finishedAt: null,
	currentBead: 'bead-alpha-001',
	lastError: null,
	attempts: 3,
	successes: 2,
	failures: 1,
	currentAttempt: {
		attemptId: 'attempt-alpha-001',
		beadId: 'bead-alpha-001',
		phase: 'executing',
		startedAt: '2026-06-01T10:05:00Z',
		elapsedMs: 45000
	},
	recentEvents: [],
	lifecycleEvents: []
};

const WORKER_LOG_ALPHA = {
	stdout: 'Booting agent harness...\nReading bead spec.\nDone.',
	stderr: ''
};

// Hub worker (from hub node) and spoke worker (from spoke node) for federation fixture.
const HUB_WORKER = {
	id: 'worker-hub-aaaaaaaa',
	kind: 'work',
	state: 'running',
	status: 'running',
	harness: 'claude',
	model: 'claude-sonnet-4-6',
	currentBead: 'bead-hub-001',
	attempts: 1,
	successes: 0,
	failures: 0,
	startedAt: '2026-06-01T10:00:00Z',
	nodeId: NODE_ID,
	nodeName: NODE_NAME
};

const SPOKE_NODE_ID = 'node-spoke-federation';
const SPOKE_WORKER = {
	id: 'worker-spoke-bbbbbbbb',
	kind: 'execute-bead',
	state: 'idle',
	status: null,
	harness: 'claude',
	model: 'claude-opus-4-8',
	currentBead: null,
	attempts: 2,
	successes: 2,
	failures: 0,
	startedAt: '2026-06-01T08:00:00Z',
	nodeId: SPOKE_NODE_ID,
	nodeName: 'spoke-node'
};

function makeWorkersResponse(workers: Record<string, unknown>[]) {
	return {
		workers: {
			edges: workers.map((w, i) => ({ node: w, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: workers.length
		}
	};
}

function makeFederationWorkersResponse(workers: Record<string, unknown>[]) {
	return {
		federatedWorkers: {
			edges: workers.map((w, i) => ({ node: w, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: workers.length
		}
	};
}

async function mockNodeWideGraphQL(
	page: import('@playwright/test').Page,
	workers: Record<string, unknown>[] = WORKERS_MULTI_PROJECT
) {
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
					data: {
						projects: {
							edges: [PROJECT_ALPHA, PROJECT_BETA].map((p) => ({ node: p }))
						}
					}
				})
			});
		} else if (body.query.includes('WorkersByProject') || body.query.includes('workersByProject')) {
			// Node-wide page may still use workersByProject per-project; pass all workers.
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (
			body.query.includes('Workers') &&
			!body.query.includes('WorkerDetail') &&
			!body.query.includes('WorkerLog') &&
			!body.query.includes('WorkerProgress')
		) {
			// Node-wide `workers` query (no projectID argument).
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse(workers) })
			});
		} else if (body.query.includes('WorkerDetail') || (body.query.includes('worker(') || body.query.includes('Worker('))) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { worker: WORKER_DETAIL_ALPHA } })
			});
		} else if (body.query.includes('WorkerLog') || body.query.includes('workerLog')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { workerLog: WORKER_LOG_ALPHA } })
			});
		} else if (body.query.includes('federation') || body.query.includes('Federation') || body.query.includes('federatedWorkers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeFederationWorkersResponse([HUB_WORKER, SPOKE_WORKER]) })
			});
		} else {
			await route.continue();
		}
	});
}

// TC-016.1 — node-wide workers single pane shows workers from multiple projects.
test('shows all local project workers in one node-wide pane', async ({ page }) => {
	await mockNodeWideGraphQL(page);
	await page.goto(`/nodes/${NODE_ID}/workers`);

	// The page heading should identify the workers workbench.
	await expect(page.getByRole('heading', { name: 'Workers', exact: true })).toBeVisible();

	// Both workers from different projects must be visible.
	// Worker from Project Alpha (running execute-bead).
	await expect(page.getByText('worker-a').first()).toBeVisible();
	// Worker from Project Beta (idle work).
	await expect(page.getByText('worker-b').first()).toBeVisible();

	// Project badges for both projects must appear.
	await expect(page.getByText(PROJECT_ALPHA.name).first()).toBeVisible();
	await expect(page.getByText(PROJECT_BETA.name).first()).toBeVisible();

	// States must be represented.
	await expect(page.getByText('running')).toBeVisible();
	await expect(page.getByText('idle')).toBeVisible();

	// Total count reflects both workers.
	await expect(page.getByText(/2 total/)).toBeVisible();

	// Current bead id should be visible for the running worker.
	await expect(page.getByText('bead-alpha-001').first()).toBeVisible();
});

// TC-016.2 — worker detail shows current bead, phase, captured output, and prompt.
test('opens worker detail with current bead and captured output', async ({ page }) => {
	await mockNodeWideGraphQL(page);
	await page.goto(`/nodes/${NODE_ID}/workers`);

	// Click the running worker row to open the detail panel.
	await page.getByText('running').first().click();

	// URL must include the worker id.
	await expect(page).toHaveURL(/\/workers\/worker-aaaaaaaa/);

	// Detail panel: current bead link must be present.
	await expect(page.getByText('bead-alpha-001').first()).toBeVisible();

	// Current phase from currentAttempt.
	await expect(page.getByText('executing')).toBeVisible();

	// Captured output from workerLog.stdout must appear.
	await expect(page.getByText('Booting agent harness...')).toBeVisible();

	// Stop action must be available for a running worker.
	await expect(page.getByRole('button', { name: 'Stop' })).toBeVisible();
});

// TC-016 federation row — federation scope listing shows hub and spoke workers.
test('shows hub and spoke workers in federation scope', async ({ page }) => {
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
					data: { projects: { edges: [PROJECT_ALPHA].map((p) => ({ node: p })) } }
				})
			});
		} else if (
			body.query.includes('federation') ||
			body.query.includes('Federation') ||
			body.query.includes('federatedWorkers') ||
			body.query.includes('scope')
		) {
			// Federation-scoped query returns both hub and spoke workers.
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: makeFederationWorkersResponse([HUB_WORKER, SPOKE_WORKER])
				})
			});
		} else if (body.query.includes('Workers') && !body.query.includes('WorkerDetail')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: makeFederationWorkersResponse([HUB_WORKER, SPOKE_WORKER])
				})
			});
		} else {
			await route.continue();
		}
	});

	// Navigate to the node-wide workers view with federation scope.
	await page.goto(`/nodes/${NODE_ID}/workers?scope=federation`);

	// Both hub and spoke workers must appear.
	await expect(page.getByText('worker-hub-a').first()).toBeVisible();
	await expect(page.getByText('worker-spoke-b').first()).toBeVisible();

	// Node badges for hub and spoke must be visible.
	await expect(page.getByText(NODE_NAME).first()).toBeVisible();
	await expect(page.getByText('spoke-node').first()).toBeVisible();

	// Total count covers both nodes.
	await expect(page.getByText(/2 total/)).toBeVisible();
});
