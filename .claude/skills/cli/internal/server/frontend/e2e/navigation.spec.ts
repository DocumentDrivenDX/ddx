import { expect, test } from '@playwright/test';

// Fixtures used across tests
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECTS = [
	{ id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' },
	{ id: 'proj-2', name: 'Project Beta', path: '/repos/beta' }
];

/**
 * Intercept /graphql and respond with mock data based on query type.
 */
async function mockGraphQL(
	page: import('@playwright/test').Page,
	nodeInfo = NODE_INFO,
	projects = PROJECTS
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: projects.map((p) => ({ node: p })) } }
				})
			});
		} else {
			await route.continue();
		}
	});
}

// TC-001: Root page redirects to /nodes/:nodeId using nodeInfo from GraphQL
test('TC-001: / redirects to /nodes/:nodeId', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');
	await expect(page).toHaveURL(/\/nodes\/node-abc/);
});

// TC-002: Nav chrome renders DDx brand and dark-mode toggle
test('TC-002: nav chrome renders DDx brand and dark-mode toggle', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');
	await expect(page.getByText('DDx')).toBeVisible();
	await expect(page.getByRole('button', { name: /toggle dark mode/i })).toBeVisible();
});

// TC-003: Nav chrome shows the node name returned by nodeInfo
test('TC-003: nav chrome shows node name', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');
	await expect(page.getByText(/Node: Test Node/).first()).toBeVisible();
});

// TC-004: Project picker populates from GraphQL Projects query
test('TC-004: project picker lists projects from GraphQL', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');

	const select = page.locator('select');
	await expect(select).toBeVisible();

	// Both project options must appear once loading is done
	await expect(select.locator('option', { hasText: 'Project Alpha' })).toBeAttached();
	await expect(select.locator('option', { hasText: 'Project Beta' })).toBeAttached();
});

// TC-005: Selecting a project navigates to /nodes/:nodeId/projects/:projectId
test('TC-005: project picker navigates to project URL on selection', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');

	const select = page.locator('select');
	await expect(select.locator('option', { hasText: 'Project Alpha' })).toBeAttached();

	await select.selectOption('proj-1');

	await expect(page).toHaveURL(/\/nodes\/node-abc\/projects\/proj-1/);
});

// TC-006: Sidebar nav links are disabled (rendered as spans) when no project is selected
test('TC-006: sidebar nav links are disabled without a project', async ({ page }) => {
	// Empty project list — no project can be selected from picker
	await mockGraphQL(page, NODE_INFO, []);
	await page.goto('/');

	// With no project, NavShell renders links as <span> elements
	const nav = page.locator('nav');
	await expect(nav.locator('span', { hasText: 'Beads' })).toBeVisible();

	// No real anchor for Beads should be present
	await expect(nav.locator('a', { hasText: 'Beads' })).toHaveCount(0);
});

// TC-007: Sidebar nav links become active anchors after a project is selected
test('TC-007: sidebar nav links activate after project selection', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');

	const select = page.locator('select');
	await expect(select.locator('option', { hasText: 'Project Alpha' })).toBeAttached();
	await select.selectOption('proj-1');

	// After selection, sidebar links should be real <a> elements
	const nav = page.locator('nav');
	await expect(nav.locator('a', { hasText: 'Beads' })).toBeVisible();
	await expect(nav.locator('a', { hasText: 'Documents' })).toBeVisible();
});
