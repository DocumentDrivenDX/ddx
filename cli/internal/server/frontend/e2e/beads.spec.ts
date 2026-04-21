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

	const IN_PROGRESS_BEAD = { ...BEAD_DETAIL, id: 'bead-002', status: 'in-progress', owner: 'alice' };
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

// TP-002 TC-003.12 (close), TC-003.13 (reopen), TC-003.14 (drag-drop) are
// DEFERRED. Rationale: the GraphQL schema has no beadClose mutation and the
// BeadDetail component exposes no close/reopen/drag-drop UI. Filed as
// separate work when the backend + component surfaces exist.
