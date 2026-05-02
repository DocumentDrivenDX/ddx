import { expect, test, type Page } from '@playwright/test';

// ddx-7e7b12f0 — regression check that the doc-graph edges and arrow marker
// resolve to the fg-muted token in both themes (light #4B5563 / dark #B8AF9C),
// not the prior border-line token whose contrast against bg-canvas failed
// WCAG AA for non-text graphics.

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT = { id: 'proj-1', name: 'Project Alpha', path: '/repos/alpha' };

const GRAPH_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT.id}/graph`;

const LIGHT_FG_MUTED = 'rgb(75, 85, 99)'; // #4B5563
const DARK_FG_MUTED = 'rgb(184, 175, 156)'; // #B8AF9C

const DOC_GRAPH_FIXTURE = {
	rootDir: '/repos/alpha',
	documents: [
		{
			id: 'doc-a',
			path: 'docs/a.md',
			title: 'Doc A',
			dependsOn: ['doc-b'],
			dependents: []
		},
		{
			id: 'doc-b',
			path: 'docs/b.md',
			title: 'Doc B',
			dependsOn: [],
			dependents: ['doc-a']
		}
	],
	pathToId: JSON.stringify({ 'docs/a.md': 'doc-a', 'docs/b.md': 'doc-b' }),
	warnings: [],
	issues: []
};

type GqlBody = { query: string; variables?: Record<string, unknown> };

async function installGraphqlMocks(page: Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as GqlBody;
		const q = body.query;

		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: [{ node: PROJECT }] } }
				})
			});
			return;
		}
		if (q.includes('DocGraph') || q.includes('docGraph')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { docGraph: DOC_GRAPH_FIXTURE } })
			});
			return;
		}
		if (q.includes('DocStale') || q.includes('docStale')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { docStale: [] } })
			});
			return;
		}
		await route.continue();
	});
}

test('doc graph edges resolve to fg-muted token in light and dark themes', async ({ page }) => {
	// Pre-load: clear any persisted dark theme so the light-theme assertion is
	// deterministic regardless of the harness's previous browser state.
	await page.addInitScript(() => {
		document.documentElement.classList.remove('dark');
	});

	await installGraphqlMocks(page);
	await page.goto(GRAPH_URL);

	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();

	const svg = page.getByTestId('doc-graph-svg');
	await expect(svg).toBeVisible();

	// AC2: at least one <line> SVG edge is rendered.
	const edges = svg.locator('line');
	await expect(edges.first()).toBeAttached();
	expect(await edges.count()).toBeGreaterThanOrEqual(1);

	const arrowPath = svg.locator('defs marker#ddx-arrow path');
	await expect(arrowPath).toBeAttached();

	// AC3: light-theme — both edge stroke and arrow marker fill resolve to
	// rgb(75, 85, 99) (#4B5563 fg-muted).
	await expect
		.poll(async () => edges.first().evaluate((el) => getComputedStyle(el).stroke))
		.toBe(LIGHT_FG_MUTED);
	await expect
		.poll(async () => arrowPath.evaluate((el) => getComputedStyle(el).fill))
		.toBe(LIGHT_FG_MUTED);

	// AC4: toggle <html>.dark and re-assert against rgb(184, 175, 156)
	// (#B8AF9C dark-fg-muted).
	await page.evaluate(() => document.documentElement.classList.add('dark'));

	await expect
		.poll(async () => edges.first().evaluate((el) => getComputedStyle(el).stroke))
		.toBe(DARK_FG_MUTED);
	await expect
		.poll(async () => arrowPath.evaluate((el) => getComputedStyle(el).fill))
		.toBe(DARK_FG_MUTED);
});
