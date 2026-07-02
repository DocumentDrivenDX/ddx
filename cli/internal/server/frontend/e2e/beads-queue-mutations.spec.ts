// TC-018: Cross-Project Bead Queue Mutations
//
// Verifies FEAT-021 US-103.
// Queue stewardship must be possible from the web UI for any registered project.
// These tests verify bead create/edit/lifecycle mutations, federation forwarding,
// and invalid-state refusals.
//
// Most of these tests MUST FAIL until the corresponding queue-mutation UI
// is implemented in the server operator workbench.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const SPOKE_NODE_ID = 'spoke-node-001';
const SPOKE_PROJECT_ID = 'proj-spoke-1';

const PROJECTS = [
	{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' },
	{ id: SPOKE_PROJECT_ID, name: 'Spoke Project', path: '/repos/spoke-alpha', nodeId: SPOKE_NODE_ID }
];

const BEADS = [
	{ id: 'bead-001', title: 'Open ready bead', status: 'open', priority: 1, labels: null },
	{ id: 'bead-002', title: 'In-progress bead', status: 'in-progress', priority: 2, labels: null }
];

const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/beads`;

function makeBeadsResponse(beads = BEADS) {
	return {
		beadsByProject: {
			edges: beads.map((b, i) => ({ node: b, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: beads.length
		}
	};
}

async function mockBase(
	page: import('@playwright/test').Page,
	opts: {
		onBeadCreate?: (vars: Record<string, unknown>) => Record<string, unknown>;
		onBeadUpdate?: (vars: Record<string, unknown>) => Record<string, unknown> | { errors: { message: string }[] };
		onLifecycle?: (query: string, vars: Record<string, unknown>) => Record<string, unknown> | { errors: { message: string }[] };
	} = {}
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		const vars = body.variables ?? {};

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('BeadsByProject') || body.query.includes('beadsByProject')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: makeBeadsResponse() }) });
		} else if (body.query.includes('query Bead(')) {
			const bead = BEADS.find((b) => b.id === vars.id) ?? BEADS[0];
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { bead: { ...bead, description: null, acceptance: null, notes: null, issueType: 'feature', owner: null, createdAt: '2026-01-01T00:00:00Z', createdBy: null, updatedAt: '2026-01-01T00:00:00Z', parent: null, dependencies: [] } } }) });
		} else if (body.query.includes('BeadCreate') || body.query.includes('beadCreate')) {
			const result = opts.onBeadCreate
				? opts.onBeadCreate(vars)
				: { id: 'bead-new', title: (vars.input as Record<string, unknown>)?.title ?? 'New bead', status: 'open', priority: 1, labels: null };
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { beadCreate: result } }) });
		} else if (body.query.includes('BeadUpdate') || body.query.includes('beadUpdate')) {
			const result = opts.onBeadUpdate ? opts.onBeadUpdate(vars) : { ...BEADS[0], ...vars };
			if ('errors' in result) {
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(result) });
			} else {
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { beadUpdate: result } }) });
			}
		} else if (
			body.query.includes('BeadApprove') || body.query.includes('beadApprove') ||
			body.query.includes('BeadBlock') || body.query.includes('beadBlock') ||
			body.query.includes('BeadCancel') || body.query.includes('beadCancel') ||
			body.query.includes('BeadReopen') || body.query.includes('beadReopen')
		) {
			if (opts.onLifecycle) {
				const result = opts.onLifecycle(body.query, vars);
				if ('errors' in result) {
					await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(result) });
				} else {
					await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: result }) });
				}
			} else {
				const statusMap: Record<string, string> = {
					beadApprove: 'ready', BeadApprove: 'ready',
					beadBlock: 'blocked', BeadBlock: 'blocked',
					beadCancel: 'cancelled', BeadCancel: 'cancelled',
					beadReopen: 'open', BeadReopen: 'open'
				};
				const key = Object.keys(statusMap).find((k) => body.query.includes(k)) ?? 'beadApprove';
				const mutName = key.charAt(0).toLowerCase() + key.slice(1);
				const result: Record<string, unknown> = {};
				result[mutName] = { ...BEADS[0], status: statusMap[key] };
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: result }) });
			}
		} else {
			await route.continue();
		}
	});
}

// TC-018.1: Creating a bead persists it in the selected project only.
test('creates a bead in the selected project only', async ({ page }) => {
	let createCalled = false;
	let createInput: Record<string, unknown> = {};

	await mockBase(page, {
		onBeadCreate: (vars) => {
			createCalled = true;
			createInput = (vars.input ?? vars) as Record<string, unknown>;
			return { id: 'bead-new', title: createInput.title as string, status: 'open', priority: 1, labels: null };
		}
	});

	await page.goto(BASE_URL);

	// Open the create form and fill required fields.
	await page.getByRole('button', { name: 'New bead' }).click();
	await expect(page.getByRole('heading', { name: /new bead/i })).toBeVisible();

	const titleInput = page.getByRole('textbox', { name: /title/i }).first();
	await titleInput.fill('Test bead for project alpha');

	await page.getByRole('button', { name: /save|create|submit/i }).click();

	// Mutation must have been called with the active project id only.
	await expect.poll(() => createCalled, { timeout: 5000 }).toBe(true);
	expect(createInput).toMatchObject({ projectId: PROJECT_ID });
	// Must NOT contain the spoke project id.
	expect(createInput.projectId).not.toBe(SPOKE_PROJECT_ID);

	// The new bead row must appear in the list.
	await expect(page.getByText('Test bead for project alpha')).toBeVisible();
});

// TC-018.2: Editing bead fields persists and refreshes the row without cross-project leakage.
test('edits bead fields and refreshes the row', async ({ page }) => {
	let updateCalled = false;
	let updateInput: Record<string, unknown> = {};

	await mockBase(page, {
		onBeadUpdate: (vars) => {
			updateCalled = true;
			updateInput = vars;
			return { ...BEADS[0], title: (vars.input as Record<string, unknown>)?.title ?? BEADS[0].title };
		}
	});

	await page.goto(`${BASE_URL}/${BEADS[0].id}`);

	// Open the edit form and change the title.
	const editBtn = page.getByRole('button', { name: /edit/i });
	await expect(editBtn).toBeVisible();
	await editBtn.click();

	const titleInput = page.getByRole('textbox', { name: /title/i }).first();
	await titleInput.fill('Updated bead title');

	await page.getByRole('button', { name: /save/i }).click();

	// beadUpdate mutation must have been called with the correct bead id.
	await expect.poll(() => updateCalled, { timeout: 5000 }).toBe(true);
	expect(updateInput).toMatchObject({ id: BEADS[0].id });

	// The updated title must appear in the UI without a full page reload.
	await expect(page.getByText('Updated bead title')).toBeVisible();
});

// TC-018.3: Lifecycle actions (approve, block, cancel, reopen) require notes and mutate state.
test('runs lifecycle actions with required notes', async ({ page }) => {
	let lifecycleMutationName = '';
	let lifecycleVars: Record<string, unknown> = {};

	await mockBase(page, {
		onLifecycle: (query, vars) => {
			if (query.includes('beadApprove') || query.includes('BeadApprove')) lifecycleMutationName = 'beadApprove';
			else if (query.includes('beadBlock') || query.includes('BeadBlock')) lifecycleMutationName = 'beadBlock';
			else if (query.includes('beadCancel') || query.includes('BeadCancel')) lifecycleMutationName = 'beadCancel';
			lifecycleVars = vars;
			return { [lifecycleMutationName]: { ...BEADS[0], status: lifecycleMutationName === 'beadApprove' ? 'ready' : 'blocked' } };
		}
	});

	await page.goto(`${BASE_URL}/${BEADS[0].id}`);

	// The lifecycle action button (e.g. Approve) must be visible.
	const approveBtn = page.getByRole('button', { name: /approve/i });
	await expect(approveBtn).toBeVisible();
	await approveBtn.click();

	// A confirmation or notes input must appear.
	const noteInput = page.getByRole('textbox', { name: /note|reason/i });
	await expect(noteInput).toBeVisible();
	await noteInput.fill('Approved after review');

	await page.getByRole('button', { name: /confirm|submit/i }).click();

	// The lifecycle mutation must have been called with a note.
	await expect.poll(() => lifecycleMutationName !== '', { timeout: 5000 }).toBe(true);
	const noteValue = lifecycleVars.note ?? lifecycleVars.reason ?? '';
	expect(String(noteValue).length).toBeGreaterThan(0);

	// The bead status must update in the UI.
	await expect(page.getByTestId('bead-status-badge')).toContainText(/ready|approved/i);
});

// TC-018.4: Hub create/edit for a spoke project forwards to the owning spoke.
test('forwards spoke bead writes from the hub', async ({ page }) => {
	let createCalled = false;
	let createInput: Record<string, unknown> = {};

	const HUB_URL = `/nodes/node-abc/projects/${SPOKE_PROJECT_ID}/beads`;

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('BeadsByProject') || body.query.includes('beadsByProject')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: makeBeadsResponse([]) }) });
		} else if (body.query.includes('BeadCreate') || body.query.includes('beadCreate')) {
			createCalled = true;
			createInput = (body.variables?.input ?? body.variables ?? {}) as Record<string, unknown>;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						beadCreate: {
							id: 'bead-spoke-new',
							title: createInput.title as string,
							status: 'open',
							priority: 1,
							labels: null,
							// Federation forwarding metadata echoed back.
							originNodeId: SPOKE_NODE_ID,
							forwardedFrom: 'node-abc'
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(HUB_URL);

	// Create a bead in the spoke project from the hub UI.
	await page.getByRole('button', { name: 'New bead' }).click();
	await expect(page.getByRole('heading', { name: /new bead/i })).toBeVisible();

	const titleInput = page.getByRole('textbox', { name: /title/i }).first();
	await titleInput.fill('Spoke bead from hub');

	await page.getByRole('button', { name: /save|create|submit/i }).click();

	// The mutation must have been called with the spoke project id.
	await expect.poll(() => createCalled, { timeout: 5000 }).toBe(true);
	expect(createInput).toMatchObject({ projectId: SPOKE_PROJECT_ID });

	// The new bead must appear, indicating it was forwarded and is visible in federated reads.
	await expect(page.getByText('Spoke bead from hub')).toBeVisible();
});

// TC-018.5: Empty required reason/note or invalid state is rejected with a visible error.
test('shows invalid state refusal', async ({ page }) => {
	await mockBase(page, {
		onLifecycle: (_query, _vars) => {
			return { errors: [{ message: 'note is required for this lifecycle action' }] };
		}
	});

	await page.goto(`${BASE_URL}/${BEADS[0].id}`);

	// Click a lifecycle action that requires a reason.
	const blockBtn = page.getByRole('button', { name: /block/i });
	await expect(blockBtn).toBeVisible();
	await blockBtn.click();

	// Submit without filling in the required reason.
	const confirmBtn = page.getByRole('button', { name: /confirm|submit/i });
	if (await confirmBtn.isVisible()) {
		await confirmBtn.click();
	}

	// An error message must appear indicating the required field.
	const errorMsg = page.locator('[data-testid="error-message"], [role="alert"]').filter({ hasText: /required|note|reason/i });
	await expect(errorMsg.first()).toBeVisible({ timeout: 5000 });
});
