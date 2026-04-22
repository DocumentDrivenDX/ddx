// FEAT-008 US-095: Operator Initiates Work from the UI
//
// These tests MUST FAIL until an Actions panel is implemented on the
// project view that dispatches Drain Queue / Re-align Specs / Run Checks
// as server-side workers. Per TDD, tests drive implementation.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}`;

const READY_QUEUE_SUMMARY = { ready: 7, blocked: 2, inProgress: 1 };

async function mockBase(page: import('@playwright/test').Page, opts: { dispatchFn?: (req: Record<string, unknown>) => Record<string, unknown> | Error } = {}) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('ProjectQueueSummary') || body.query.includes('queueSummary')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { queueSummary: READY_QUEUE_SUMMARY } }) });
		} else if (body.query.includes('WorkerDispatch') || body.query.includes('workerDispatch')) {
			if (opts.dispatchFn) {
				const result = opts.dispatchFn(body.variables ?? {});
				if (result instanceof Error) {
					await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ errors: [{ message: result.message }] }) });
					return;
				}
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { workerDispatch: result } }) });
				return;
			}
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { workerDispatch: { id: 'worker-new-123', state: 'queued' } } }) });
		} else {
			await route.continue();
		}
	});
}

test('US-095.a: project view exposes an Actions panel with Drain / Align / Checks buttons', async ({ page }) => {
	await mockBase(page);
	await page.goto(BASE_URL);

	const panel = page.getByRole('region', { name: /actions/i });
	await expect(panel, 'Actions panel must be visible on the project overview').toBeVisible();

	for (const label of [/drain queue/i, /re-?align specs/i, /run checks/i]) {
		await expect(panel.getByRole('button', { name: label })).toBeVisible();
	}
});

test('US-095.b: Drain queue opens a confirmation dialog showing ready bead count', async ({ page }) => {
	await mockBase(page);
	await page.goto(BASE_URL);

	await page.getByRole('button', { name: /drain queue/i }).click();

	const dialog = page.getByRole('dialog', { name: /drain queue/i });
	await expect(dialog).toBeVisible();
	await expect(dialog.getByText(/7 ready beads/i)).toBeVisible();
	await expect(dialog.getByRole('button', { name: /confirm|start/i })).toBeVisible();
	await expect(dialog.getByRole('button', { name: /cancel/i })).toBeVisible();
});

test('US-095.c: confirming dispatches a worker and surfaces it in the list', async ({ page }) => {
	let dispatched: Record<string, unknown> | null = null;
	await mockBase(page, {
		dispatchFn: (req) => {
			dispatched = req;
			return { id: 'worker-drain-001', state: 'queued', kind: 'execute-loop' };
		}
	});

	await page.goto(BASE_URL);
	await page.getByRole('button', { name: /drain queue/i }).click();
	const dialog = page.getByRole('dialog', { name: /drain queue/i });
	await dialog.getByRole('button', { name: /confirm|start/i }).click();

	await expect.poll(() => dispatched).not.toBeNull();
	expect(dispatched).toMatchObject({ kind: 'execute-loop' });

	// Originating button should anchor to the new worker's detail page within 1s.
	const link = page.getByRole('link', { name: /worker-drain-001/ });
	await expect(link).toBeVisible({ timeout: 2000 });
});

test('US-095.d: dispatch errors are surfaced with remediation, not silent', async ({ page }) => {
	await mockBase(page, {
		dispatchFn: () => new Error('queue already has an active execute-loop worker')
	});

	await page.goto(BASE_URL);
	await page.getByRole('button', { name: /drain queue/i }).click();
	await page.getByRole('dialog', { name: /drain queue/i }).getByRole('button', { name: /confirm|start/i }).click();

	const alert = page.getByRole('alert');
	await expect(alert).toBeVisible();
	await expect(alert).toContainText(/active execute-loop worker/);
});

test('US-095.e: disabled action surfaces the prerequisite reason in a tooltip', async ({ page }) => {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('ProjectQueueSummary') || body.query.includes('queueSummary')) {
			// Zero ready beads → Drain queue is disabled.
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { queueSummary: { ready: 0, blocked: 0, inProgress: 0 } } }) });
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);
	const drainBtn = page.getByRole('button', { name: /drain queue/i });
	await expect(drainBtn).toBeDisabled();
	await drainBtn.hover();
	await expect(page.getByRole('tooltip')).toContainText(/no ready beads/i);
});
