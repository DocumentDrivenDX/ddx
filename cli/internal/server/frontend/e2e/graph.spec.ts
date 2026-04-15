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

function makeGraphResponse(docs = GRAPH_DOCS, warnings: string[] = []) {
	return {
		docGraph: {
			rootDir: '/repos/alpha',
			documents: docs,
			warnings
		}
	};
}

/**
 * Set up GraphQL route mocking for the graph page.
 */
async function mockGraphQL(
	page: import('@playwright/test').Page,
	docs = GRAPH_DOCS,
	warnings: string[] = []
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
				body: JSON.stringify({ data: makeGraphResponse(docs, warnings) })
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

// TC-034: Graph warnings are displayed in the warning banner
test('TC-034: graph warnings appear in the warning banner', async ({ page }) => {
	const warnings = [
		'Circular dependency detected: doc-001 → doc-002 → doc-001',
		'Orphaned document: docs/orphan.md'
	];
	await mockGraphQL(page, GRAPH_DOCS, warnings);
	await page.goto(BASE_URL);

	await expect(page.getByText('Circular dependency detected: doc-001 → doc-002 → doc-001')).toBeVisible();
	await expect(page.getByText('Orphaned document: docs/orphan.md')).toBeVisible();
});

// TC-035: No warning banner when graph has no warnings
test('TC-035: warning banner is absent when no warnings are returned', async ({ page }) => {
	await mockGraphQL(page, GRAPH_DOCS, []);
	await page.goto(BASE_URL);

	// The warning container should not be present
	await expect(page.locator('.bg-amber-50, .bg-amber-950')).toHaveCount(0);
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
