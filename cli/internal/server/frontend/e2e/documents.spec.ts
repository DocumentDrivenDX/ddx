import { expect, test } from '@playwright/test';

// Shared fixtures
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/documents`;

const DOCUMENTS = [
	{ id: 'doc-001', path: 'docs/helix/01-frame/vision.md', title: 'Vision' },
	{ id: 'doc-002', path: 'docs/helix/01-frame/prd.md', title: 'PRD' },
	{ id: 'doc-003', path: 'docs/helix/02-design/api.md', title: 'API Design' }
];

const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

function makeDocsResponse(docs = DOCUMENTS, totalCount = DOCUMENTS.length) {
	return {
		documents: {
			edges: docs.map((d, i) => ({ node: d, cursor: `cursor-${i}` })),
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount
		}
	};
}

/**
 * Set up GraphQL and API route mocking for the documents pages.
 */
async function mockRoutes(page: import('@playwright/test').Page) {
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
		} else if (body.query.includes('Documents')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeDocsResponse() })
			});
		} else {
			await route.continue();
		}
	});

	// Mock the document content REST endpoint
	await page.route('/api/documents/**', async (route) => {
		const url = route.request().url();
		const path = url.replace(/.*\/api\/documents\//, '');
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ path, content: `# Content of ${path}\n\nThis is mock content.` })
		});
	});
}

// TC-020: Documents page loads with heading and document table
test('TC-020: documents page loads with heading and document table', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Documents' })).toBeVisible();

	// Table headers should be present
	await expect(page.getByRole('columnheader', { name: 'Title' })).toBeVisible();
	await expect(page.getByRole('columnheader', { name: 'Path' })).toBeVisible();
});

// TC-021: Document list renders all documents from the GraphQL response
test('TC-021: document list renders all returned documents', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(BASE_URL);

	await expect(page.getByText('Vision')).toBeVisible();
	await expect(page.getByText('PRD')).toBeVisible();
	await expect(page.getByText('API Design')).toBeVisible();
});

// TC-022: Total count is displayed in the header area
test('TC-022: documents page shows total count', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(BASE_URL);

	// e.g. "3 total"
	await expect(page.getByText(/3 total/)).toBeVisible();
});

// TC-023: Document paths are rendered in the path column
test('TC-023: document paths are shown in the table', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(BASE_URL);

	await expect(page.getByText('docs/helix/01-frame/vision.md')).toBeVisible();
	await expect(page.getByText('docs/helix/01-frame/prd.md')).toBeVisible();
});

// TC-024: Clicking a document row navigates to the document detail page
test('TC-024: clicking a document row navigates to the detail page', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(BASE_URL);

	// Click on the Vision document row
	await page.getByText('Vision').click();

	// URL should include the document path
	await expect(page).toHaveURL(/\/documents\/docs\/helix\/01-frame\/vision\.md/);
});

// TC-025: Document detail page loads content from the REST API
test('TC-025: document detail page fetches and displays document content', async ({ page }) => {
	await mockRoutes(page);
	await page.goto(`${BASE_URL}/docs/helix/01-frame/vision.md`);

	// The mock content starts with "# Content of"
	await expect(page.getByText(/Content of docs\/helix\/01-frame\/vision\.md/)).toBeVisible();
});

// TC-026: Document detail edit button opens editor and Save triggers DocumentWrite mutation
test('TC-026: editing a document fires the DocumentWrite mutation', async ({ page }) => {
	let mutationCalled = false;
	let mutationInput: { path?: string; content?: string } = {};

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
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
		} else if (body.query.includes('DocumentWrite') || body.query.includes('documentWrite')) {
			mutationCalled = true;
			mutationInput = (body.variables ?? {}) as { path?: string; content?: string };
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { documentWrite: { path: 'docs/helix/01-frame/vision.md' } } })
			});
		} else {
			await route.continue();
		}
	});

	await page.route('/api/documents/**', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({
				path: 'docs/helix/01-frame/vision.md',
				content: '# Vision\n\nOriginal content.'
			})
		});
	});

	await page.goto(`${BASE_URL}/docs/helix/01-frame/vision.md`);

	// Edit button should be visible once content is rendered
	await expect(page.getByRole('button', { name: /edit/i })).toBeVisible();
	await page.getByRole('button', { name: /edit/i }).click();

	// Textarea should appear with the document content
	const textarea = page.locator('textarea');
	await expect(textarea).toBeVisible();
	await textarea.fill('# Vision\n\nUpdated content.');

	// Click Save
	await page.getByRole('button', { name: /save/i }).click();

	// The DocumentWrite mutation must have been called
	expect(mutationCalled).toBe(true);
	expect(mutationInput.path).toBe('docs/helix/01-frame/vision.md');
});

// TC-027: Empty state shown when no documents are returned
test('TC-027: documents page shows empty state when no documents are returned', async ({ page }) => {
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
		} else if (body.query.includes('Documents')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeDocsResponse([], 0) })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	await expect(page.getByText('No documents found.')).toBeVisible();
	await expect(page.getByText('0 total')).toBeVisible();
});
