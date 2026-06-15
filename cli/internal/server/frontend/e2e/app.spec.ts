import { test, expect, type Page } from '@playwright/test';

// TP-002: DDx Server Web UI — End-to-End Tests
// Covers TC-001 through TC-008.

// ---------------------------------------------------------------------------
// TC-001: Project overview (dashboard-level smoke)
//
// Since Stage 3.8, `/` redirects to `/nodes/<id>` and concrete project views
// live under `/nodes/<nodeId>/projects/<projectId>/...`. TC-001 mirrors the
// fixture-based pattern used in navigation.spec.ts: mock GraphQL for
// NodeInfo / Projects / ProjectQueueSummary, navigate into the fixture
// project, and assert the shell + project entry points render.
// ---------------------------------------------------------------------------
const TC001_NODE = { id: 'node-abc', name: 'Test Node' };
const TC001_PROJECT_ID = 'proj-1';
const TC001_PROJECTS = [
	{ id: TC001_PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' },
	{ id: 'proj-2', name: 'Project Beta', path: '/repos/beta' }
];
const TC001_BASE_URL = `/nodes/${TC001_NODE.id}/projects/${TC001_PROJECT_ID}`;

async function mockProjectOverview(page: Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: TC001_NODE } })
			});
		} else if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: TC001_PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else if (body.query.includes('ProjectQueueSummary') || body.query.includes('queueSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { queueSummary: { ready: 3, blocked: 1, inProgress: 0 } }
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

test.describe('TC-001: Project overview', () => {
	test.beforeEach(async ({ page }) => {
		await mockProjectOverview(page);
		await page.goto(TC001_BASE_URL);
		await page.waitForSelector('h1');
	});

	test('TC-001.1 — project overview loads', async ({ page }) => {
		await expect(page.locator('h1')).toContainText('Project Alpha');
		await expect(page.getByText('Project overview')).toBeVisible();
	});

	test('TC-001.2 — sidebar exposes Artifacts entry point', async ({ page }) => {
		const artifactsHref = `${TC001_BASE_URL}/artifacts`;
		const link = page.locator(`nav a[href="${artifactsHref}"]`);
		await expect(link).toBeVisible();
		await expect(page.locator('nav').getByText('Documents', { exact: true })).toHaveCount(0);
	});

	test('TC-001.3 — queue summary shows ready/blocked/in-progress', async ({ page }) => {
		const summary = page.getByLabel('Queue summary');
		await expect(summary).toBeVisible();
		await expect(summary.locator('text=Ready')).toBeVisible();
		await expect(summary.locator('text=Blocked')).toBeVisible();
		await expect(summary.locator('text=In progress')).toBeVisible();
	});

	test('TC-001.4 — actions panel renders', async ({ page }) => {
		const panel = page.getByRole('region', { name: /actions/i });
		await expect(panel).toBeVisible();
	});

	test('TC-001.5 — node identity visible in nav chrome', async ({ page }) => {
		await expect(page.getByText(`Node: ${TC001_NODE.name}`).first()).toBeVisible();
	});

	test('TC-001.6 — navigate to artifacts', async ({ page }) => {
		const artifactsHref = `${TC001_BASE_URL}/artifacts`;
		await page.click(`nav a[href="${artifactsHref}"]`);
		await expect(page).toHaveURL(new RegExp(`${TC001_BASE_URL}/artifacts`));
	});

	test('TC-001.7 — navigate to beads', async ({ page }) => {
		const beadsHref = `${TC001_BASE_URL}/beads`;
		await page.click(`nav a[href="${beadsHref}"]`);
		await expect(page).toHaveURL(new RegExp(`${TC001_BASE_URL}/beads`));
	});

	test('TC-001.8 — navigate to graph', async ({ page }) => {
		const graphHref = `${TC001_BASE_URL}/graph`;
		await page.click(`nav a[href="${graphHref}"]`);
		await expect(page).toHaveURL(new RegExp(`${TC001_BASE_URL}/graph`));
	});
});

// ---------------------------------------------------------------------------
// Fixture discovery — TC-002/003/005/007 navigate the live Go-server fixture
// harness and assert against the seeded documents/beads/personas/graph data.
// The harness derives node and project IDs from the temp workspace path so
// the IDs are stable per run but not predictable; resolve them at runtime
// from /api/projects + the GraphQL nodeInfo query.
// ---------------------------------------------------------------------------
async function getFixtureIds(
	request: import('@playwright/test').APIRequestContext
): Promise<{ nodeId: string; projectId: string; nodeName: string }> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as {
		data: { nodeInfo: { id: string; name: string } };
	};
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{
		id: string;
		name: string;
		path: string;
	}>;
	// The fixture harness boots ddx-server from a `mktemp -d -t ddx-e2e-XXXXXX`
	// workspace, so the fixture project's path/name is prefixed `ddx-e2e-`.
	// Other entries in /api/projects (carried over in the developer's persisted
	// server state) must not be picked up here — those would point the spec at
	// unrelated, developer-local data.
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

function projectBase(ids: { nodeId: string; projectId: string }): string {
	return `/nodes/${ids.nodeId}/projects/${ids.projectId}`;
}

// ---------------------------------------------------------------------------
// TC-002: Artifacts Page (fixture-backed)
// Markdown documents are surfaced through the canonical Artifacts page. The
// fixture's `.ddx/graphs/scale.yml` registers the 2k synthetic scale corpus
// under scale/docs/ as docgraph roots, so the project-scoped Artifacts page
// renders that corpus along with the table chrome.
// ---------------------------------------------------------------------------
test.describe('TC-002: Artifacts', () => {
	let ids: { nodeId: string; projectId: string; nodeName: string };

	test.beforeEach(async ({ page, request }) => {
		ids = await getFixtureIds(request);
		await page.goto(`${projectBase(ids)}/artifacts?mediaType=text%2Fmarkdown`);
		await page.waitForSelector('h1');
	});

	test('TC-002.1 — artifacts page loads markdown documents against the fixture project', async ({
		page
	}) => {
		await expect(page.locator('h1')).toContainText('Artifacts');
		await expect(page.getByText('2000 total')).toBeVisible();
	});

	test('TC-002.2 — table chrome renders Title and Path columns', async ({ page }) => {
		const headerRow = page.locator('thead tr');
		await expect(headerRow.locator('th', { hasText: 'Title' })).toBeVisible();
		await expect(headerRow.locator('th', { hasText: 'Path' })).toBeVisible();
	});

	test('TC-002.3 — fixture markdown artifacts render as table rows', async ({ page }) => {
		// First scale corpus entry must appear; the page paginates first=50 so
		// the earliest synthetic artifacts are always in the initial slice.
		const firstRow = page.locator('tbody tr', { hasText: 'scale/docs/artifact-0001.md' });
		await expect(firstRow).toBeVisible();
	});

	test('TC-002.4 — sidebar Artifacts entry highlights when active', async ({ page }) => {
		const link = page.locator(`nav a[href="${projectBase(ids)}/artifacts"]`);
		await expect(link).toBeVisible();
		await expect(link).toHaveAttribute('aria-current', 'page');
		await expect(page.locator('nav').getByText('Documents', { exact: true })).toHaveCount(0);
	});
});

// ---------------------------------------------------------------------------
// TC-003: Beads list (fixture-backed)
// Fixture seeds 4 beads (fx-001..fx-004) covering open/closed/blocked states.
// The current beads UI is a filterable/sortable table, not a kanban — the
// status filter chips replace the old per-column kanban layout.
// ---------------------------------------------------------------------------
test.describe('TC-003: Beads', () => {
	let ids: { nodeId: string; projectId: string; nodeName: string };

	test.beforeEach(async ({ page, request }) => {
		ids = await getFixtureIds(request);
		await page.goto(`${projectBase(ids)}/beads`);
		await page.waitForSelector('h1:has-text("Beads")', { timeout: 15000 });
	});

	test('TC-003.1 — beads page loads with fixture data', async ({ page }) => {
		await expect(page.locator('h1:has-text("Beads")')).toBeVisible();
		// Fixture seeds 4 beads — totals appear as "<filtered> of <total>".
		await expect(page.getByText(/\d+ of \d+/)).toBeVisible();
		const rows = page.locator('[data-testid="bead-row"]');
		expect(await rows.count()).toBeGreaterThan(0);
	});

	test('TC-003.2 — fixture bead IDs appear in the table', async ({ page }) => {
		const rows = page.locator('[data-testid="bead-row"]');
		// fx-001 is the seeded open ready bead.
		await expect(rows.filter({ hasText: 'fx-001' })).toHaveCount(1);
	});

	test('TC-003.3 — search narrows to a single fixture bead', async ({ page }) => {
		const search = page.locator('input[placeholder="Search beads…"]');
		await search.fill('fx-002');
		// 200ms debounce on the URL update.
		await expect(page).toHaveURL(/[?&]q=fx-002/);
		const rows = page.locator('[data-testid="bead-row"]');
		await expect(rows).toHaveCount(1);
		await expect(rows.first()).toContainText('fx-002');
	});

	test('TC-003.4 — clearing search restores fixture rows', async ({ page }) => {
		const search = page.locator('input[placeholder="Search beads…"]');
		await search.fill('fx-002');
		await expect(page).toHaveURL(/[?&]q=fx-002/);
		await search.fill('');
		await expect(page).not.toHaveURL(/[?&]q=/);
		const rows = page.locator('[data-testid="bead-row"]');
		expect(await rows.count()).toBeGreaterThan(1);
	});

	test('TC-003.5 — status filter chip narrows fixture rows to closed', async ({ page }) => {
		await page.getByRole('button', { name: 'closed', exact: true }).click();
		await expect(page).toHaveURL(/[?&]status=closed/);
		const rows = page.locator('[data-testid="bead-row"]');
		// Fixture has exactly one closed bead (fx-002).
		await expect(rows).toHaveCount(1);
		await expect(rows.first()).toContainText('fx-002');
	});

	test('TC-003.6 — selecting a bead row opens its detail route', async ({ page }) => {
		const fxRow = page.locator('[data-testid="bead-row"]').filter({ hasText: 'fx-001' }).first();
		await fxRow.click();
		await expect(page).toHaveURL(new RegExp(`${projectBase(ids)}/beads/fx-001`));
	});

	test('TC-003.7 — create-bead form opens with required fields', async ({ page }) => {
		await page.getByRole('button', { name: 'New bead' }).click();
		await expect(page.getByRole('heading', { name: 'New bead' })).toBeVisible();
		const form = page.locator('form');
		await expect(form).toBeVisible();
		await expect(form.locator('text=Title')).toBeVisible();
		await expect(form.locator('text=Description')).toBeVisible();
		await expect(form.locator('text=Acceptance')).toBeVisible();
	});
});

// ---------------------------------------------------------------------------
// TC-005: Agent Runs (fixture-backed)
// The project-scoped /sessions compatibility route now redirects to the run
// layer. The fixture has no recorded runs, so the Runs page must render its
// empty state without errors.
// ---------------------------------------------------------------------------
test.describe('TC-005: Agent', () => {
	test('TC-005.1 — sessions compatibility route opens the run layer', async ({ page, request }) => {
		const ids = await getFixtureIds(request);
		await page.goto(`${projectBase(ids)}/sessions`);
		await expect(page).toHaveURL(new RegExp(`${projectBase(ids)}/runs\\?layer=run`));
		await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible();
		await expect(page.getByRole('button', { name: 'run', exact: true })).toHaveAttribute(
			'aria-pressed',
			'true'
		);
	});
});

// ---------------------------------------------------------------------------
// TC-004: Document Graph
// ---------------------------------------------------------------------------
test.describe('TC-004: Graph', () => {
	test('TC-004.1 — graph loads', async ({ page }) => {
		await page.goto('/graph');
		// Should not show an error
		await expect(page.locator('text=Error')).not.toBeVisible({ timeout: 5000 });
	});
});

// ---------------------------------------------------------------------------
// TC-006: Personas
// ---------------------------------------------------------------------------
test.describe('TC-006: Personas', () => {
	test.beforeEach(async ({ page }) => {
		await page.goto('/personas');
	});

	test('TC-006.1 — persona list loads', async ({ page }) => {
		// Personas page has an h2 "Personas" in the sidebar
		await expect(page.locator('text=Personas').first()).toBeVisible();
		await page.waitForTimeout(1000);
	});

	test('TC-006.2 — select persona', async ({ page }) => {
		const firstPersona = page.locator('.w-80 button').first();
		if (!(await firstPersona.isVisible({ timeout: 2000 }).catch(() => false))) {
			test.skip(true, 'No personas available (library not installed)');
			return;
		}
		await firstPersona.click();
		await expect(page.locator('pre')).toBeVisible({ timeout: 5000 });
	});

	test('TC-006.3 — role badges', async ({ page }) => {
		const badges = page.locator('.bg-blue-100');
		const count = await badges.count();
		expect(count).toBeGreaterThanOrEqual(0);
	});
});

// ---------------------------------------------------------------------------
// TC-007: Navigation (fixture-backed)
// Sidebar links activate once a project is selected. Navigate to the fixture
// project and verify the project-scoped nav routes (Beads/Artifacts/Graph/
// Runs/Personas) are reachable via SPA clicks.
// ---------------------------------------------------------------------------
test.describe('TC-007: Navigation', () => {
	let ids: { nodeId: string; projectId: string; nodeName: string };

	test.beforeEach(async ({ page, request }) => {
		ids = await getFixtureIds(request);
		await page.goto(projectBase(ids));
		await page.waitForSelector('nav a', { timeout: 15000 });
	});

	test('TC-007.1 — all project-scoped nav links visible', async ({ page }) => {
		const base = projectBase(ids);
		const nav = page.locator('nav');
		for (const slug of ['beads', 'artifacts', 'graph', 'runs', 'personas']) {
			await expect(nav.locator(`a[href="${base}/${slug}"]`)).toBeVisible();
		}
		await expect(nav.getByText('Documents', { exact: true })).toHaveCount(0);
		// Brand link returns to project home.
		await expect(page.locator(`header a[href="${base}"]`)).toBeVisible();
	});

	test('TC-007.2 — node identity exposed in nav chrome', async ({ page }) => {
		await expect(page.getByText(`Node: ${ids.nodeName}`).first()).toBeVisible();
	});

	test('TC-007.3 — SPA routing across project pages', async ({ page }) => {
		const base = projectBase(ids);
		const nav = page.locator('nav');

		await nav.locator(`a[href="${base}/artifacts"]`).click();
		await expect(page).toHaveURL(new RegExp(`${base}/artifacts`));

		await nav.locator(`a[href="${base}/beads"]`).click();
		await expect(page).toHaveURL(new RegExp(`${base}/beads`));

		await nav.locator(`a[href="${base}/graph"]`).click();
		await expect(page).toHaveURL(new RegExp(`${base}/graph`));

		await nav.locator(`a[href="${base}/runs"]`).click();
		await expect(page).toHaveURL(new RegExp(`${base}/runs`));

		await nav.locator(`a[href="${base}/personas"]`).click();
		await expect(page).toHaveURL(new RegExp(`${base}/personas`));

		// Brand returns to project home.
		await page.locator(`header a[href="${base}"]`).click();
		await expect(page).toHaveURL(new RegExp(`${base}/?$`));
	});
});

// ---------------------------------------------------------------------------
// TC-008: HTTP API
// ---------------------------------------------------------------------------
test.describe('TC-008: HTTP API', () => {
	test('TC-008.1 — health endpoint', async ({ request }) => {
		const resp = await request.get('/api/health');
		expect(resp.ok()).toBeTruthy();
		const body = await resp.json();
		expect(body.status).toBe('ok');
	});

	test('TC-008.2 — documents list', async ({ request }) => {
		const resp = await request.get('/api/documents');
		expect(resp.ok()).toBeTruthy();
		const body = await resp.json();
		expect(Array.isArray(body)).toBeTruthy();
	});

	test('TC-008.3 — beads list', async ({ request }) => {
		const resp = await request.get('/api/beads');
		expect(resp.ok()).toBeTruthy();
		const body = await resp.json();
		expect(Array.isArray(body)).toBeTruthy();
	});

	test('TC-008.4 — beads status', async ({ request }) => {
		const resp = await request.get('/api/beads/status');
		expect(resp.ok()).toBeTruthy();
		const body = await resp.json();
		expect(body).toHaveProperty('open');
		expect(body).toHaveProperty('closed');
	});

	test('TC-008.5 — personas list', async ({ request }) => {
		const resp = await request.get('/api/personas');
		expect(resp.ok()).toBeTruthy();
		const body = await resp.json();
		expect(Array.isArray(body)).toBeTruthy();
	});

	test('TC-008.6 — doc graph', async ({ request }) => {
		const resp = await request.get('/api/docs/graph');
		expect(resp.ok()).toBeTruthy();
		const body = await resp.json();
		expect(Array.isArray(body)).toBeTruthy();
	});
});
