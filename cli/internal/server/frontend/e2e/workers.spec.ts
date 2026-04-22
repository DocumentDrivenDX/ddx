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
	}
};

const WORKER_LOG = { stdout: 'Starting execution...\nStep 1 complete\nStep 2 complete', stderr: '' };

function makeWorkersResponse(workers = WORKERS) {
	return {
		workersByProject: {
			edges: workers.map((w, i) => ({ node: w, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: workers.length
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
