import { expect, test } from '@playwright/test';

// Shared fixtures
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/beads`;

const BEADS = [
	{ id: 'bead-001', title: 'First bead', status: 'open', priority: 1, labels: ['ddx', 'ui'] },
	{ id: 'bead-002', title: 'Second bead', status: 'in-progress', priority: 2, labels: ['ddx'] },
	{ id: 'bead-003', title: 'Blocked bead', status: 'blocked', priority: 3, labels: null }
];

const PAGE_INFO_NO_NEXT = { hasNextPage: false, endCursor: null };
const PAGE_INFO_HAS_NEXT = { hasNextPage: true, endCursor: 'cursor-page-2' };

function makeBeadsResponse(beads = BEADS, pageInfo = PAGE_INFO_NO_NEXT, totalCount = BEADS.length) {
	return {
		beadsByProject: {
			edges: beads.map((b, i) => ({ node: b, cursor: `cursor-${i}` })),
			pageInfo,
			totalCount
		}
	};
}

const BEAD_DETAIL = {
	id: 'bead-001',
	title: 'First bead',
	status: 'open',
	priority: 1,
	issueType: 'feature',
	owner: 'alice',
	createdAt: '2026-01-01T00:00:00Z',
	createdBy: 'alice',
	updatedAt: '2026-01-02T00:00:00Z',
	labels: ['ddx', 'ui'],
	parent: null,
	description: 'A test description',
	acceptance: 'Must pass tests',
	notes: null,
	dependencies: []
};

const CREATED_BEAD = {
	id: 'bead-new',
	title: 'New bead from test',
	status: 'open',
	priority: 2,
	issueType: 'feature',
	owner: null,
	createdAt: '2026-01-03T00:00:00Z',
	createdBy: null,
	updatedAt: '2026-01-03T00:00:00Z',
	labels: null,
	parent: null,
	description: null,
	acceptance: null,
	notes: null,
	dependencies: []
};

const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

/**
 * Set up GraphQL route mocking for the beads pages.
 */
async function mockGraphQL(page: import('@playwright/test').Page) {
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
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse() })
			});
		} else if (body.query.includes('query Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { bead: BEAD_DETAIL } })
			});
		} else if (body.query.includes('BeadCreate') || body.query.includes('beadCreate')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { beadCreate: CREATED_BEAD } })
			});
		} else if (body.query.includes('BeadLifecycle') || body.query.includes('beadLifecycle')) {
			// Subscriptions over HTTP are not expected; pass through
			await route.continue();
		} else {
			await route.continue();
		}
	});
}

// TC-010: Beads page loads and displays heading and bead list
test('TC-010: beads page loads with heading and bead table', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Beads' })).toBeVisible();
	await expect(page.getByText('First bead')).toBeVisible();
	await expect(page.getByText('Second bead')).toBeVisible();
	await expect(page.getByText('Blocked bead')).toBeVisible();
});

// TC-011: Total count is shown in the header area
test('TC-011: beads page shows bead count', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// "3 of 3" count display
	await expect(page.getByText(/\d+ of \d+/)).toBeVisible();
});

// TC-012: Status filter chips render and can be activated
test('TC-012: status filter chips are rendered and toggle active state', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// All four status chips should be visible
	const openChip = page.getByRole('button', { name: 'open' });
	const inProgressChip = page.getByRole('button', { name: 'in-progress' });
	await expect(openChip).toBeVisible();
	await expect(inProgressChip).toBeVisible();

	// Clicking a chip updates the URL with status param
	await openChip.click();
	await expect(page).toHaveURL(/[?&]status=open/);
});

// TC-013: Search input filters beads via URL query param
test('TC-013: search input debounces and updates URL query param', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	const searchInput = page.locator('input[type="search"]');
	await expect(searchInput).toBeVisible();

	await searchInput.fill('First');
	// After debounce (200ms) the URL should reflect the search term
	await expect(page).toHaveURL(/[?&]q=First/, { timeout: 2000 });
});

// TC-014: Clicking a bead row navigates to the bead detail panel
test('TC-014: clicking a bead row opens its detail panel', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await page.getByText('First bead').click();

	// URL should now include the beadId
	await expect(page).toHaveURL(/\/beads\/bead-001/);
	// Detail panel content should appear
	await expect(page.getByRole('heading', { name: 'First bead' })).toBeVisible({ timeout: 1000 });
	await expect(page.getByText('A test description')).toBeVisible();
});

// TC-015: "New bead" button opens the create form and submits a BeadCreate mutation
test('TC-015: new bead form opens, fills, and submits BeadCreate mutation', async ({ page }) => {
	let mutationCalled = false;

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
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse() })
			});
		} else if (body.query.includes('BeadCreate') || body.query.includes('beadCreate')) {
			mutationCalled = true;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { beadCreate: CREATED_BEAD } })
			});
		} else if (body.query.includes('query Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { bead: CREATED_BEAD } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	// Open the create form
	await page.getByRole('button', { name: 'New bead' }).click();
	await expect(page.getByRole('heading', { name: 'New bead' })).toBeVisible();

	// Fill in the title field
	const titleInput = page.getByRole('textbox', { name: /title/i }).first();
	await titleInput.fill('New bead from test');

	// Submit the form
	await page.getByRole('button', { name: /save|create|submit/i }).click();

	// The mutation should have been called
	expect(mutationCalled).toBe(true);
});

// TC-016: Empty state is shown when no beads are returned
test('TC-016: beads page shows empty state when no beads are returned', async ({ page }) => {
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
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse([], PAGE_INFO_NO_NEXT, 0) })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	await expect(page.getByText('No beads found.')).toBeVisible();
});

// TC-017: "Load more" button appears when hasNextPage is true
test('TC-017: load more button appears and triggers second-page fetch', async ({ page }) => {
	const PAGE_2_BEAD = {
		id: 'bead-page2',
		title: 'Page two bead',
		status: 'open',
		priority: 4,
		labels: null
	};

	let callCount = 0;
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
		} else if (body.query.includes('BeadsByProject')) {
			callCount++;
			if (callCount === 1) {
				await route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ data: makeBeadsResponse(BEADS, PAGE_INFO_HAS_NEXT, 4) })
				});
			} else {
				await route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({
						data: makeBeadsResponse([PAGE_2_BEAD], PAGE_INFO_NO_NEXT, 4)
					})
				});
			}
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	// Load more button should be visible
	const loadMoreButton = page.getByRole('button', { name: /load more/i });
	await expect(loadMoreButton).toBeVisible();

	// Click load more and check second page results appear
	await loadMoreButton.click();
	await expect(page.getByText('Page two bead')).toBeVisible();
});

// TP-002 TC-003.11 — unclaim an in-progress bead from the detail panel.
// Covers FEAT-008 bead lifecycle mutation wiring for the Unclaim path
// (beadUnclaim GraphQL mutation; BeadDetail.svelte Unclaim button).
test('TC-003.11: Unclaim button on in-progress bead fires BeadUnclaim mutation', async ({
	page
}) => {
	let unclaimCalled = false;

	const IN_PROGRESS_BEAD = {
		...BEAD_DETAIL,
		id: 'bead-002',
		status: 'in-progress',
		owner: 'alice'
	};
	const UNCLAIMED_BEAD = { ...IN_PROGRESS_BEAD, status: 'open', owner: null };

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
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse([IN_PROGRESS_BEAD, BEADS[0], BEADS[2]]) })
			});
		} else if (body.query.includes('query Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { bead: IN_PROGRESS_BEAD } })
			});
		} else if (body.query.includes('BeadUnclaim') || body.query.includes('beadUnclaim')) {
			unclaimCalled = true;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { beadUnclaim: UNCLAIMED_BEAD } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/bead-002`);

	// An in-progress bead shows the Unclaim button, not Claim.
	const unclaimButton = page.getByRole('button', { name: /unclaim/i });
	await expect(unclaimButton).toBeVisible();

	await unclaimButton.click();

	// Mutation must fire and the UI must reflect the transition.
	await expect.poll(() => unclaimCalled, { timeout: 5000 }).toBe(true);
});

// TP-002 TC-003.12 (generic close), TC-003.13 (reopen), TC-003.14 (drag-drop)
// are DEFERRED. US-085c below covers the delete soft-close path via beadClose;
// generic close/reopen/drag-drop controls remain separate lifecycle work.

// -----------------------------------------------------------------------
// FEAT-008 US-082g: sort + filter the beads list
// These tests MUST FAIL until the implementation provides the UI affordances
// called out in the AC. Do not weaken them to match gaps.
// -----------------------------------------------------------------------

const FILTER_BEADS = [
	{ id: 'bead-p0-a', title: 'Urgent thing', status: 'open', priority: 0, labels: ['ui'] },
	{
		id: 'bead-p0-b',
		title: 'Another urgent',
		status: 'ready',
		priority: 0,
		labels: ['ui', 'infra']
	},
	{ id: 'bead-p1', title: 'Normal work', status: 'open', priority: 1, labels: ['docs'] },
	{ id: 'bead-p3', title: 'Cleanup', status: 'closed', priority: 3, labels: ['chore'] },
	{
		id: 'bead-block',
		title: 'Waiting on upstream',
		status: 'blocked',
		priority: 0,
		labels: ['agent']
	}
];

test('US-082g.a: priority sort defaults to P0-first and is toggleable', async ({ page }) => {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse(FILTER_BEADS) })
			});
		} else if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else {
			await route.continue();
		}
	});
	await page.goto(BASE_URL);

	const prioritySort = page.getByRole('button', { name: /sort by priority/i });
	await expect(prioritySort, 'priority sort control must be visible').toBeVisible();

	// Default order: P0 before P1 before P3.
	const rows = page.locator('[data-testid="bead-row"]');
	await expect(rows.first()).toHaveAttribute('data-priority', '0');

	await prioritySort.click();
	// Toggled: P3 first, P0 last.
	await expect(rows.first()).toHaveAttribute('data-priority', /[23]/);
});

test('US-082g.b: status filter chip narrows list and updates URL', async ({ page }) => {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('BeadsByProject')) {
			const url = new URL(page.url());
			const statusParam = url.searchParams.get('status');
			const filtered = statusParam
				? FILTER_BEADS.filter((b) => b.status === statusParam)
				: FILTER_BEADS;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse(filtered) })
			});
		} else if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);
	const blockedChip = page.getByRole('button', { name: /blocked/i });
	await expect(blockedChip).toBeVisible();
	await blockedChip.click();

	await expect(page).toHaveURL(/status=blocked/);
	await expect(page.getByText('Waiting on upstream')).toBeVisible();
	await expect(page.getByText('Urgent thing')).toHaveCount(0);
});

test('US-082g.c: multi-filter URL is bookmarkable', async ({ page }) => {
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
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else if (body.query.includes('BeadsByProject')) {
			const url = new URL(page.url());
			let filtered = FILTER_BEADS;
			if (url.searchParams.get('status'))
				filtered = filtered.filter((b) => b.status === url.searchParams.get('status'));
			if (url.searchParams.get('priority'))
				filtered = filtered.filter((b) => String(b.priority) === url.searchParams.get('priority'));
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse(filtered) })
			});
		} else {
			await route.continue();
		}
	});

	// Open with filters already in URL — page MUST render matching beads on load.
	await page.goto(`${BASE_URL}?status=open&priority=0`);

	const statusChip = page.getByRole('button', { name: /open/i }).first();
	const priorityChip = page.getByRole('button', { name: /p0/i }).first();
	await expect(statusChip).toHaveAttribute('aria-pressed', 'true');
	await expect(priorityChip).toHaveAttribute('aria-pressed', 'true');

	await expect(page.getByText('Urgent thing')).toBeVisible();
	await expect(page.getByText('Cleanup')).toHaveCount(0);
});

test('US-082g.d: label chip filter is clickable from a row', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	const uiLabel = page
		.locator('[data-testid="bead-row"] [data-testid="label-chip"]', { hasText: /ui/i })
		.first();
	await expect(uiLabel).toBeVisible();
	await uiLabel.click();

	await expect(page).toHaveURL(/labels?=ui/);
});

test('US-082g.e: empty filter result shows zero-state with clear-filters affordance', async ({
	page
}) => {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse([]) })
			});
		} else if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}?status=open&priority=0&label=nonexistent`);
	await expect(page.getByText(/no beads match/i)).toBeVisible();
	const clearBtn = page.getByRole('button', { name: /clear filters/i });
	await expect(clearBtn).toBeVisible();
	await clearBtn.click();
	await expect(page).not.toHaveURL(/status=/);
});

// -----------------------------------------------------------------------
// FEAT-008 US-085c: delete (soft-close) a bead from the UI
// -----------------------------------------------------------------------

test('US-085c: delete button triggers typed-confirmation modal and BeadClose mutation', async ({
	page
}) => {
	let closeCalled = false;
	let closeArgs: Record<string, unknown> | null = null;
	const CHILD_BEAD = { id: 'bead-child', parent: BEAD_DETAIL.id };

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
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
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse() })
			});
		} else if (body.query.includes('query Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						bead: BEAD_DETAIL,
						projectBeads: {
							edges: [{ node: { id: BEAD_DETAIL.id, parent: null } }, { node: CHILD_BEAD }]
						}
					}
				})
			});
		} else if (body.query.includes('BeadClose') || body.query.includes('beadClose')) {
			closeCalled = true;
			closeArgs = body.variables ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { beadClose: { ...BEAD_DETAIL, status: 'closed' } } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/${BEAD_DETAIL.id}`);

	const deleteBtn = page.getByRole('button', { name: 'Delete', exact: true });
	await expect(deleteBtn).toBeVisible();
	await deleteBtn.click();

	// Confirmation modal requires typing the bead ID before enabling Confirm.
	let modal = page.getByRole('dialog', { name: /delete bead/i });
	await expect(modal).toBeVisible();
	await expect(modal.getByRole('checkbox', { name: /cascade to child beads/i })).toBeVisible();
	await modal.getByRole('button', { name: 'Cancel', exact: true }).click();
	await expect(deleteBtn).toBeFocused();

	await deleteBtn.click();
	modal = page.getByRole('dialog', { name: /delete bead/i });
	const confirmBtn = modal.getByRole('button', { name: 'Delete bead', exact: true });
	await expect(confirmBtn).toBeDisabled();

	const idField = modal.getByLabel(/type the bead id/i);
	await idField.fill(BEAD_DETAIL.id);
	await expect(confirmBtn).toBeEnabled();

	await confirmBtn.click();
	await expect.poll(() => closeCalled).toBe(true);
	expect(closeArgs).toMatchObject({
		id: BEAD_DETAIL.id,
		reason: 'deleted via UI'
	});

	// URL should redirect to list; detail panel should be gone.
	await expect(page).toHaveURL(new RegExp(`${BASE_URL}/?$`));
});

// -----------------------------------------------------------------------
// Detail header: long bead ids truncate cleanly and do not overlap actions
// -----------------------------------------------------------------------

const LONG_ID = '.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1';
const LONG_BEAD_DETAIL = { ...BEAD_DETAIL, id: LONG_ID };
const LONG_BEAD_ROW = {
	id: LONG_ID,
	title: 'First bead',
	status: 'open',
	priority: 1,
	labels: ['ddx', 'ui']
};

async function mockLongIdGraphQL(
	page: import('@playwright/test').Page,
	overrideId?: string
) {
	const beadId = overrideId ?? LONG_ID;
	const row = { ...LONG_BEAD_ROW, id: beadId };
	const detail = { ...LONG_BEAD_DETAIL, id: beadId };
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
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse([row], PAGE_INFO_NO_NEXT, 1) })
			});
		} else if (body.query.includes('query Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { bead: detail } })
			});
		} else {
			await route.continue();
		}
	});
}

test('detail header: long bead id truncates, exposes full id on hover, and does not overlap action buttons', async ({
	page,
	context
}) => {
	await context.grantPermissions(['clipboard-read', 'clipboard-write']);
	await mockLongIdGraphQL(page);
	await page.goto(`${BASE_URL}/${encodeURIComponent(LONG_ID)}`);

	const idEl = page.getByTestId('bead-detail-id');
	await expect(idEl).toBeVisible();
	await expect(idEl).toHaveText(LONG_ID);

	// AC 1 + 2: truncate class + title attribute
	await expect(idEl).toHaveClass(/truncate/);
	await expect(idEl).toHaveClass(/min-w-0/);
	await expect(idEl).toHaveAttribute('title', LONG_ID);

	// AC 4: Delete button remains visible and clickable; its rect does not intersect the id span.
	const deleteBtn = page.getByRole('button', { name: 'Delete', exact: true });
	await expect(deleteBtn).toBeVisible();

	const idBox = await idEl.boundingBox();
	const delBox = await deleteBtn.boundingBox();
	expect(idBox).not.toBeNull();
	expect(delBox).not.toBeNull();
	// Rects must not overlap (id must end before delete button begins).
	expect(idBox!.x + idBox!.width).toBeLessThanOrEqual(delBox!.x);

	// Click should not throw (button is fully clickable, not covered).
	await deleteBtn.click();
	await expect(page.getByRole('dialog', { name: /delete bead/i })).toBeVisible();
	await page.keyboard.press('Escape');

	// AC 5: id rendered width ≤ parent container width minus action-button group width.
	const parent = idEl.locator('xpath=..');
	const parentBox = await parent.boundingBox();
	// The action-button group is the sibling of parent; grab it via the Delete button's
	// closest flex container.
	const actionGroup = deleteBtn.locator('xpath=..');
	const actionBox = await actionGroup.boundingBox();
	expect(parentBox).not.toBeNull();
	expect(actionBox).not.toBeNull();
	expect(idBox!.width).toBeLessThanOrEqual(parentBox!.width);
	// Combined id + action group fits within the panel's width.
	const panel = page.locator('div.max-w-xl').first();
	const panelBox = await panel.boundingBox();
	expect(panelBox).not.toBeNull();
	expect(idBox!.width + actionBox!.width).toBeLessThanOrEqual(panelBox!.width);

	// AC 5: copy button writes full id to clipboard.
	const copyBtn = page.getByTestId('bead-detail-copy-id');
	await expect(copyBtn).toBeVisible();
	await copyBtn.click();
	const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
	expect(clipboardText).toBe(LONG_ID);
});

// -----------------------------------------------------------------------
// Dependency DAG view: parent + child/depends-on relationships rendered
// in the bead detail panel.
// -----------------------------------------------------------------------

test('bead detail renders dependency DAG: parent and child/depends-on relationships are visible', async ({
	page
}) => {
	const PARENT_ID = 'bead-parent-001';
	const CHILD_ID = 'bead-child-001';
	const DAG_BEAD_DETAIL = {
		id: CHILD_ID,
		title: 'Child bead with parent and deps',
		status: 'open',
		priority: 1,
		issueType: 'feature',
		owner: 'alice',
		createdAt: '2026-01-01T00:00:00Z',
		createdBy: 'alice',
		updatedAt: '2026-01-02T00:00:00Z',
		labels: null,
		parent: PARENT_ID,
		description: null,
		acceptance: null,
		notes: null,
		dependencies: [
			{
				issueId: CHILD_ID,
				dependsOnId: 'bead-dep-A',
				type: 'blocks',
				createdAt: null,
				createdBy: null
			},
			{
				issueId: CHILD_ID,
				dependsOnId: 'bead-dep-B',
				type: 'relates',
				createdAt: null,
				createdBy: null
			}
		]
	};
	const DAG_BEAD_ROW = {
		id: CHILD_ID,
		title: DAG_BEAD_DETAIL.title,
		status: 'open',
		priority: 1,
		labels: null
	};

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
		} else if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeBeadsResponse([DAG_BEAD_ROW], PAGE_INFO_NO_NEXT, 1) })
			});
		} else if (body.query.includes('query Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { bead: DAG_BEAD_DETAIL } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/${CHILD_ID}`);

	// Parent relationship is rendered in the detail panel.
	const parentTerm = page.getByText('Parent', { exact: true });
	await expect(parentTerm).toBeVisible();
	await expect(page.getByText(PARENT_ID)).toBeVisible();

	// Child/depends-on relationships are rendered in the Dependencies block.
	const depsTerm = page.getByText('Dependencies', { exact: true });
	await expect(depsTerm).toBeVisible();
	await expect(page.getByText('bead-dep-A')).toBeVisible();
	await expect(page.getByText('bead-dep-B')).toBeVisible();
	// The dependency type/edge label is rendered alongside each node id.
	await expect(page.getByText('(blocks)')).toBeVisible();
	await expect(page.getByText('(relates)')).toBeVisible();
});

test('detail header: renders without horizontal scroll or clipped action buttons for id lengths 8, 32, 60, 120', async ({
	page
}) => {
	const LENGTHS = [8, 32, 60, 120];
	for (const len of LENGTHS) {
		const id = 'b' + 'x'.repeat(len - 1);
		await page.unroute('/graphql').catch(() => {});
		await mockLongIdGraphQL(page, id);
		await page.goto(`${BASE_URL}/${encodeURIComponent(id)}`);

		const idEl = page.getByTestId('bead-detail-id');
		await expect(idEl).toBeVisible();
		const deleteBtn = page.getByRole('button', { name: 'Delete', exact: true });
		await expect(deleteBtn).toBeVisible();

		// No horizontal overflow inside the panel.
		const panel = page.locator('div.max-w-xl').first();
		const overflow = await panel.evaluate((el) => el.scrollWidth - el.clientWidth);
		expect(overflow).toBeLessThanOrEqual(1);

		// id span must not overlap delete button.
		const idBox = await idEl.boundingBox();
		const delBox = await deleteBtn.boundingBox();
		expect(idBox!.x + idBox!.width).toBeLessThanOrEqual(delBox!.x);
	}
});
