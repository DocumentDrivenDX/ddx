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
	await expect(page.getByRole('link', { name: 'DDx' })).toBeVisible();
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

/**
 * Resolve the harness-derived fixture node + project IDs at runtime by
 * querying GraphQL nodeInfo and /api/projects. The fixture harness boots
 * ddx-server from a `mktemp -d -t ddx-e2e-XXXXXX` workspace, so the fixture
 * project's path/name is prefixed `ddx-e2e-`. Other entries in /api/projects
 * (carried over in the developer's persisted server state) must not be
 * picked up here — those would point the spec at unrelated, developer-local
 * data.
 */
async function getFixtureIds(
	request: import('@playwright/test').APIRequestContext
): Promise<{ nodeId: string; projectId: string; nodeName: string }> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as {
		data: { nodeInfo: { id: string; name: string } }
	};
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{
		id: string;
		name: string;
		path: string;
	}>;
	const fixture = projects.find((p) => /(^|\/)ddx-e2e-/.test(p.path) || /^ddx-e2e-/.test(p.name));
	if (!fixture) {
		throw new Error(
			`fixture server has no ddx-e2e-* project registered (got: ${projects
				.map((p) => p.id)
				.join(', ')})`
		);
	}
	return {
		nodeId: nodeBody.data.nodeInfo.id,
		projectId: fixture.id,
		nodeName: nodeBody.data.nodeInfo.name
	};
}

// TC-006: Sidebar nav links are disabled (rendered as spans) when no project is selected
test('TC-006: sidebar nav links are disabled without a project', async ({ page }) => {
	// Hit the live fixture harness without selecting a project. The picker
	// does not auto-select, so projectStore stays empty and NavShell renders
	// project-scoped links as <span> elements.
	await page.goto('/');
	await page.waitForSelector('nav');

	// The project-scoped Beads entry is visible…
	const nav = page.locator('nav');
	await expect(nav.getByText('Beads', { exact: true })).toBeVisible();

	// …and is rendered as a <span>, not an <a>. Exact-label matching is
	// required so the always-anchor "All Beads" node-scoped link below the
	// divider is not picked up here.
	const beadsAnchor = nav.locator('a').filter({ hasText: /^\s*Beads\s*$/ });
	await expect(beadsAnchor).toHaveCount(0);
});

// TC-007: Sidebar nav links become active anchors after a project is selected
test('TC-007: sidebar nav links activate after project selection', async ({ page, request }) => {
	const ids = await getFixtureIds(request);
	await page.goto('/');

	const select = page.locator('select[aria-label="Project"]');
	// Wait for the picker to finish loading the real fixture project list.
	await expect(select).toBeEnabled();
	await expect(select.locator(`option[value="${ids.projectId}"]`)).toBeAttached();
	await select.selectOption(ids.projectId);

	// After selection, sidebar links should be real <a> elements pointing at
	// the fixture project's routes.
	const base = `/nodes/${ids.nodeId}/projects/${ids.projectId}`;
	const nav = page.locator('nav');
	await expect(nav.locator(`a[href="${base}/beads"]`)).toBeVisible();
	await expect(nav.locator(`a[href="${base}/documents"]`)).toBeVisible();
});

/**
 * Extended mock that also answers the project overview page's GraphQL needs.
 */
async function mockGraphQLForOverview(page: import('@playwright/test').Page) {
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
		} else if (body.query.includes('ProjectQueueSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { queueSummary: { ready: 2, blocked: 1, inProgress: 0 } }
				})
			});
		} else if (body.query.includes('beadsByProject') || body.query.includes('beads(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						beadsByProject: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						},
						beads: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						}
					}
				})
			});
		} else {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: {} })
			});
		}
	});
}

// TC-008: Overview sidebar entry routes to project home and DDx brand is a link
test('TC-008: overview entry and DDx brand return to project home', async ({ page }) => {
	await mockGraphQLForOverview(page);
	await page.goto('/nodes/node-abc/projects/proj-1/beads');

	const nav = page.locator('nav');
	const overview = nav.locator('a', { hasText: 'Overview' });
	await expect(overview).toBeVisible();
	await overview.click();

	await expect(page).toHaveURL(/\/nodes\/node-abc\/projects\/proj-1\/?$/);
	await expect(page.getByText('Project overview')).toBeVisible();
	await expect(page.getByLabel('Queue summary')).toBeVisible();

	// Now go into a sub-page and click the DDx brand to return home.
	await page.goto('/nodes/node-abc/projects/proj-1/sessions');
	const brand = page.locator('header a', { hasText: 'DDx' });
	await expect(brand).toBeVisible();
	await brand.click();
	await expect(page).toHaveURL(/\/nodes\/node-abc\/projects\/proj-1\/?$/);
});
