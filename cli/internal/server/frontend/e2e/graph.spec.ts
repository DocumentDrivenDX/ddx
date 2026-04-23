import { expect, test } from '@playwright/test';

// Shared fixtures
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/graph`;

const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

const GRAPH_DOCS = [
	{
		id: 'doc-001',
		path: 'docs/vision.md',
		title: 'Vision',
		dependsOn: [],
		dependents: ['doc-002', 'doc-003']
	},
	{
		id: 'doc-002',
		path: 'docs/prd.md',
		title: 'PRD',
		dependsOn: ['doc-001'],
		dependents: ['doc-003']
	},
	{
		id: 'doc-003',
		path: 'docs/design.md',
		title: 'Design',
		dependsOn: ['doc-001', 'doc-002'],
		dependents: []
	}
];

interface GraphIssueFixture {
	kind: string;
	path: string | null;
	id: string | null;
	message: string;
	relatedPath: string | null;
}

function makeGraphResponse(
	docs = GRAPH_DOCS,
	warnings: string[] = [],
	issues: GraphIssueFixture[] = []
) {
	return {
		docGraph: {
			rootDir: '/repos/alpha',
			documents: docs,
			warnings,
			issues
		}
	};
}

/**
 * Set up GraphQL route mocking for the graph page.
 */
async function mockGraphQL(
	page: import('@playwright/test').Page,
	docs = GRAPH_DOCS,
	warnings: string[] = [],
	issues: GraphIssueFixture[] = []
) {
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
		} else if (body.query.includes('DocGraph')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeGraphResponse(docs, warnings, issues) })
			});
		} else {
			await route.continue();
		}
	});
}

// TC-030: Graph page loads with heading
test('TC-030: graph page loads with Document Graph heading', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();
});

// TC-031: Node and edge counts are displayed in the header
test('TC-031: graph page shows node and edge counts', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// 3 nodes, 3 edges (doc-002 depends on doc-001 = 1 edge, doc-003 depends on doc-001 + doc-002 = 2 edges)
	await expect(page.getByText(/3 nodes/)).toBeVisible();
	await expect(page.getByText(/3 edges/)).toBeVisible();
});

// TC-032: D3Graph canvas element is rendered when documents exist
test('TC-032: D3Graph SVG element is rendered for non-empty graph', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// The D3Graph component renders an SVG
	await expect(page.locator('svg')).toBeVisible();
});

// TC-033: Empty state is shown when no documents are in the graph
test('TC-033: graph page shows empty state when no documents', async ({ page }) => {
	await mockGraphQL(page, [], []);
	await page.goto(BASE_URL);

	await expect(page.getByText('No documents in graph.')).toBeVisible();

	// Node/edge counts should be 0 · 0
	await expect(page.getByText(/0 nodes/)).toBeVisible();
	await expect(page.getByText(/0 edges/)).toBeVisible();
});

// TC-034: Structured issues are surfaced in the integrity panel.
test('TC-034: structured issue messages appear in the integrity panel', async ({ page }) => {
	const issues: GraphIssueFixture[] = [
		{
			kind: 'cycle',
			path: null,
			id: null,
			message: 'cycle detected: doc-001 -> doc-002 -> doc-001',
			relatedPath: null
		},
		{
			kind: 'missing_dep',
			path: 'docs/orphan.md',
			id: 'ghost',
			message: 'document "docs/orphan.md" declares dependency "ghost" which is not in the graph',
			relatedPath: null
		}
	];
	await mockGraphQL(page, GRAPH_DOCS, [], issues);
	await page.goto(BASE_URL);

	// Expand groups to reveal messages.
	await page.getByTestId('integrity-group-cycle').click();
	await page.getByTestId('integrity-group-missing_dep').click();

	await expect(page.getByText('cycle detected: doc-001 -> doc-002 -> doc-001')).toBeVisible();
	await expect(
		page.getByText(
			'document "docs/orphan.md" declares dependency "ghost" which is not in the graph'
		)
	).toBeVisible();
});

// TC-035: No amber surface when graph has no issues
test('TC-035: integrity surface is absent when no issues are returned', async ({ page }) => {
	await mockGraphQL(page, GRAPH_DOCS, [], []);
	await page.goto(BASE_URL);

	// The integrity panel container should not be present
	await expect(page.getByTestId('integrity-panel')).toHaveCount(0);
});

// TC-037: Integrity panel groups structured issues by kind with counts.
test('TC-037: integrity panel groups issues by kind with counts and paths', async ({ page }) => {
	const issues: GraphIssueFixture[] = [
		{
			kind: 'duplicate_id',
			path: 'docs/alpha.md',
			id: 'shared.id',
			message: 'duplicate document id "shared.id" in "docs/alpha.md"',
			relatedPath: 'docs/beta.md'
		},
		{
			kind: 'missing_dep',
			path: 'docs/gamma.md',
			id: 'ghost.doc',
			message: 'document "doc.gamma" declares dependency "ghost.doc" which is not in the graph',
			relatedPath: null
		}
	];

	await mockGraphQL(page, GRAPH_DOCS, [], issues);
	await page.goto(BASE_URL);

	const panel = page.getByTestId('integrity-panel');
	await expect(panel).toBeVisible();
	await expect(panel).toContainText('Duplicate ID');
	await expect(panel).toContainText('(1)');
	await expect(panel).toContainText('Missing dep target');

	const badge = page.getByTestId('integrity-badge');
	await expect(badge).toBeVisible();
	await expect(badge).toContainText('2');

	// Expand Duplicate ID group and assert both paths are visible.
	await page.getByTestId('integrity-group-duplicate_id').click();
	await expect(panel).toContainText('docs/alpha.md');
	await expect(panel).toContainText('docs/beta.md');

	// Clicking the path link navigates to the documents route for that file.
	const pathLink = panel.getByTestId('integrity-path-link').first();
	const href = await pathLink.getAttribute('href');
	expect(href).toBe(`/nodes/node-abc/projects/${PROJECT_ID}/documents/docs/alpha.md`);

	// Follow the link to assert routing. Override the GraphQL mock to stub
	// DocumentByPath so SvelteKit's client-side router can render the target.
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('DocumentByPath')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						documentByPath: {
							id: 'shared.id',
							path: 'docs/alpha.md',
							title: 'Alpha',
							content: '# Alpha',
							dependsOn: [],
							inputs: [],
							dependents: []
						}
					}
				})
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
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
		} else {
			await route.continue();
		}
	});
	await pathLink.click();
	await expect(page).toHaveURL(href!);
});

// TC-038: Clean graph hides both the badge and the integrity panel.
test('TC-038: clean graph hides the integrity badge and panel', async ({ page }) => {
	await mockGraphQL(page, GRAPH_DOCS, [], []);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();
	await expect(page.getByTestId('integrity-panel')).toHaveCount(0);
	await expect(page.getByTestId('integrity-badge')).toHaveCount(0);
});

// TC-036: Graph page re-fetches DocGraph query on navigation (interaction with query)
test('TC-036: graph page issues DocGraph query to load graph data', async ({ page }) => {
	let graphQueryCount = 0;

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
		} else if (body.query.includes('DocGraph')) {
			graphQueryCount++;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeGraphResponse() })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	// Wait for the page to fully render
	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();

	// DocGraph query must have been called at least once to populate the page
	expect(graphQueryCount).toBeGreaterThanOrEqual(1);
});
