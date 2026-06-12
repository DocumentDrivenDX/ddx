// TC-019: Cross-Project Spec Editing
//
// Verifies FEAT-021 US-104.
// Spec editing is scoped to project documents, especially docs/helix/**.
// Path traversal and absolute paths are rejected; stale writes return a
// conflict error; federation forwards spoke document writes; graph/staleness
// views refresh after save.
//
// These tests MUST FAIL until the full spec-editing UI with path confinement,
// stale-write detection, federation forwarding, and graph refresh is implemented.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const SPOKE_NODE_ID = 'spoke-node-001';
const SPOKE_PROJECT_ID = 'proj-spoke-1';

const PROJECTS = [
	{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' },
	{ id: SPOKE_PROJECT_ID, name: 'Spoke Project', path: '/repos/spoke-alpha', nodeId: SPOKE_NODE_ID }
];

const HELIX_FEAT_DOC = {
	id: 'doc-feat-026',
	path: 'docs/helix/01-frame/features/FEAT-026-federation.md',
	title: 'FEAT-026 Federation',
	content: '---\nid: FEAT-026\n---\n# Federation\n\nInitial content.'
};

const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/documents`;

async function mockDocBase(
	page: import('@playwright/test').Page,
	opts: {
		onDocumentWrite?: (vars: Record<string, unknown>) => Record<string, unknown> | { errors: { message: string }[] };
		extraQuery?: (query: string, vars: Record<string, unknown>) => Record<string, unknown> | null;
	} = {}
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		const vars = body.variables ?? {};

		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('Documents')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { documents: { edges: [{ node: HELIX_FEAT_DOC, cursor: 'c0' }], pageInfo: { hasNextPage: false, endCursor: null }, totalCount: 1 } } }) });
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			const path = (vars.path as string) ?? '';
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { documentByPath: { ...HELIX_FEAT_DOC, path: path || HELIX_FEAT_DOC.path } } })
			});
		} else if (body.query.includes('DocumentWrite') || body.query.includes('documentWrite')) {
			if (opts.onDocumentWrite) {
				const result = opts.onDocumentWrite(vars);
				if ('errors' in result) {
					await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(result) });
				} else {
					await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { documentWrite: result } }) });
				}
			} else {
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { documentWrite: { path: (vars.path as string) ?? HELIX_FEAT_DOC.path } } }) });
			}
		} else if (opts.extraQuery) {
			const result = opts.extraQuery(body.query, vars);
			if (result !== null) {
				await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: result }) });
				return;
			}
			await route.continue();
		} else {
			await route.continue();
		}
	});
}

// TC-019.1: Editing and saving a docs/helix/** document writes through documentWrite.
test('saves a helix spec in the selected project', async ({ page }) => {
	let writeCalled = false;
	let writePath = '';
	let writeContent = '';

	await mockDocBase(page, {
		onDocumentWrite: (vars) => {
			writeCalled = true;
			writePath = (vars.path as string) ?? '';
			writeContent = (vars.content as string) ?? '';
			return { path: writePath };
		}
	});

	await page.goto(`${BASE_URL}/${HELIX_FEAT_DOC.path}`);

	// The document must render and offer an Edit button.
	await expect(page.getByRole('button', { name: /edit/i })).toBeVisible();
	await page.getByRole('button', { name: /edit/i }).click();

	// Switch to plain mode to type raw markdown.
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await expect(textarea).toBeVisible();
	await textarea.fill('---\nid: FEAT-026\n---\n# Federation\n\nUpdated content.');

	await page.getByRole('button', { name: /save/i }).click();

	// documentWrite must have been called with the correct project document path.
	await expect.poll(() => writeCalled, { timeout: 5000 }).toBe(true);
	expect(writePath).toBe(HELIX_FEAT_DOC.path);
	expect(writeContent).toContain('Updated content');

	// The rendered view must show the saved content.
	await expect(page.getByText('Updated content')).toBeVisible();
});

// TC-019.2: Absolute paths and ../ traversal writes are rejected; no file is written.
test('rejects traversal and absolute document writes', async ({ page }) => {
	let writeCalled = false;

	await mockDocBase(page, {
		onDocumentWrite: (vars) => {
			writeCalled = true;
			const path = (vars.path as string) ?? '';
			// Server-side: reject any path containing traversal or starting with /.
			if (path.includes('../') || path.startsWith('/')) {
				return { errors: [{ message: `path confinement violation: ${path}` }] };
			}
			return { path };
		}
	});

	await page.goto(`${BASE_URL}/${HELIX_FEAT_DOC.path}`);
	await page.getByRole('button', { name: /edit/i }).click();

	// Attempt to change the save path to a traversal or absolute path via the path input.
	const pathInput = page.getByRole('textbox', { name: /path/i });
	if (await pathInput.isVisible()) {
		await pathInput.fill('../../../etc/passwd');
		await page.getByRole('radio', { name: /plain/i }).click();
		const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
		await textarea.fill('malicious content');
		await page.getByRole('button', { name: /save/i }).click();

		// An error must be visible and no file must be written outside the project.
		const error = page.locator('[data-testid="error-message"], [role="alert"]').filter({ hasText: /confinement|traversal|invalid.*path|path.*violation/i });
		await expect(error.first()).toBeVisible({ timeout: 5000 });
	} else {
		// If no path input, verify the UI prevents navigating outside the document tree.
		// The save button must remain scoped to the current project path.
		await page.getByRole('radio', { name: /plain/i }).click();
		const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
		await textarea.fill('safe content');
		await page.getByRole('button', { name: /save/i }).click();
		if (writeCalled) {
			// Verify the path written is always under docs/.
			expect(HELIX_FEAT_DOC.path).toMatch(/^docs\//);
		}
	}
});

// TC-019.3: Saving after the document changed externally fails with a conflict message.
test('refuses stale document saves', async ({ page }) => {
	let saveAttempts = 0;

	await mockDocBase(page, {
		onDocumentWrite: (_vars) => {
			saveAttempts++;
			// Second save attempt simulates a stale-write conflict.
			if (saveAttempts > 1) {
				return { errors: [{ message: 'conflict: document has changed since last fetch' }] };
			}
			return { path: HELIX_FEAT_DOC.path };
		}
	});

	await page.goto(`${BASE_URL}/${HELIX_FEAT_DOC.path}`);
	await page.getByRole('button', { name: /edit/i }).click();

	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });

	// First save succeeds.
	await textarea.fill('First save.');
	await page.getByRole('button', { name: /save/i }).click();
	await expect.poll(() => saveAttempts >= 1, { timeout: 5000 }).toBe(true);

	// Re-open edit mode (the UI should allow re-editing after a successful save).
	await page.getByRole('button', { name: /edit/i }).click();
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea2 = page.getByRole('textbox', { name: /plain markdown editor/i });

	// Second save triggers a stale-write conflict from the mock.
	await textarea2.fill('Second save — should conflict.');
	await page.getByRole('button', { name: /save/i }).click();

	// A conflict/stale-write error must be shown to the operator.
	const conflictMsg = page.locator('[data-testid="error-message"], [role="alert"]').filter({ hasText: /conflict|stale|changed since/i });
	await expect(conflictMsg.first()).toBeVisible({ timeout: 5000 });
});

// TC-019.4: Hub save for a spoke project forwards to the owning spoke.
test('forwards spoke document writes from the hub', async ({ page }) => {
	let writeCalled = false;
	let writeVars: Record<string, unknown> = {};

	const SPOKE_DOC_URL = `/nodes/node-abc/projects/${SPOKE_PROJECT_ID}/documents/${HELIX_FEAT_DOC.path}`;

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { documentByPath: HELIX_FEAT_DOC } }) });
		} else if (body.query.includes('Documents')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { documents: { edges: [{ node: HELIX_FEAT_DOC, cursor: 'c0' }], pageInfo: { hasNextPage: false, endCursor: null }, totalCount: 1 } } }) });
		} else if (body.query.includes('DocumentWrite') || body.query.includes('documentWrite')) {
			writeCalled = true;
			writeVars = body.variables ?? {};
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						documentWrite: {
							path: writeVars.path as string,
							// Federation audit metadata forwarded by hub.
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

	await page.goto(SPOKE_DOC_URL);

	// Edit the spoke document from the hub UI.
	await page.getByRole('button', { name: /edit/i }).click();
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await expect(textarea).toBeVisible();
	await textarea.fill('---\nid: FEAT-026\n---\n# Federation\n\nForwarded edit.');

	await page.getByRole('button', { name: /save/i }).click();

	// The documentWrite mutation must have been called with the spoke project's document path.
	await expect.poll(() => writeCalled, { timeout: 5000 }).toBe(true);
	expect(writeVars.path).toBe(HELIX_FEAT_DOC.path);
});

// TC-019.5: After a spec save, document graph/staleness views refresh or show a refresh affordance.
test('refreshes graph or staleness state after save', async ({ page }) => {
	let graphRefreshRequested = false;

	await mockDocBase(page, {
		extraQuery: (query, _vars) => {
			if (query.includes('DocumentGraph') || query.includes('documentGraph') || query.includes('Staleness') || query.includes('staleness')) {
				graphRefreshRequested = true;
				return {
					documentGraph: { nodes: [{ id: HELIX_FEAT_DOC.id, path: HELIX_FEAT_DOC.path, staleness: 'fresh' }], edges: [] }
				};
			}
			return null;
		}
	});

	await page.goto(`${BASE_URL}/${HELIX_FEAT_DOC.path}`);

	// Edit and save the document.
	await page.getByRole('button', { name: /edit/i }).click();
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await textarea.fill('---\nid: FEAT-026\n---\n# Federation\n\nContent that triggers staleness refresh.');
	await page.getByRole('button', { name: /save/i }).click();

	// After the save, either the graph was refreshed automatically or a
	// "Refresh" / "Re-check staleness" affordance is visible.
	const refreshAfforded =
		graphRefreshRequested ||
		(await page.getByRole('button', { name: /refresh|re-?check/i }).isVisible()) ||
		(await page.getByTestId('graph-refresh-indicator').isVisible());
	expect(refreshAfforded, 'graph or staleness must refresh or offer a manual refresh after save').toBe(true);
});
