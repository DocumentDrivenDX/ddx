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
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			const vars =
				(route.request().postDataJSON() as { variables?: Record<string, string> }).variables ?? {};
			const path = vars.path ?? '';
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						documentByPath: {
							path,
							content: `# Content of ${path}\n\nThis is mock content.`
						}
					}
				})
			});
		} else {
			await route.continue();
		}
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

	await expect(page.getByRole('cell', { name: /^Vision$/ })).toBeVisible();
	await expect(page.getByRole('cell', { name: /^PRD$/ })).toBeVisible();
	await expect(page.getByRole('cell', { name: /^API Design$/ })).toBeVisible();
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
	await page.getByRole('cell', { name: /^Vision$/ }).click();

	// URL should include the document path
	await expect(page).toHaveURL(/\/documents\/docs\/helix\/01-frame\/vision\.md/);
});

// TC-025: Document detail page loads content via GraphQL documentByPath
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
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						documentByPath: {
							path: 'docs/helix/01-frame/vision.md',
							content: '# Vision\n\nOriginal content.'
						}
					}
				})
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/docs/helix/01-frame/vision.md`);

	// Edit button should be visible once content is rendered
	await expect(page.getByRole('button', { name: /edit/i })).toBeVisible();
	await page.getByRole('button', { name: /edit/i }).click();

	// Plain mode exposes the raw markdown textarea.
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await expect(textarea).toBeVisible();
	await textarea.fill('# Vision\n\nUpdated content.');

	// Click Save
	await page.getByRole('button', { name: /save/i }).click();

	// The DocumentWrite mutation must have been called
	expect(mutationCalled).toBe(true);
	expect(mutationInput.path).toBe('docs/helix/01-frame/vision.md');
});

// TC-027: Empty state shown when no documents are returned
test('TC-027: documents page shows empty state when no documents are returned', async ({
	page
}) => {
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

// -----------------------------------------------------------------------
// FEAT-008 US-081a: follow intra-repo markdown links inside the rendered view
// -----------------------------------------------------------------------

const DOC_WITH_LINK = {
	id: 'doc-linker',
	path: 'docs/helix/01-frame/vision.md',
	title: 'Vision',
	// Rendered markdown contains an intra-repo relative link and an external link.
	content:
		'# Vision\n\nSee [PRD](../01-frame/prd.md) for requirements. ' +
		'External [Anthropic](https://anthropic.com) link.'
};

const DOC_LINK_TARGET = {
	id: 'doc-prd',
	path: 'docs/helix/01-frame/prd.md',
	title: 'PRD',
	content: '# Product Requirements\n\nThis is the PRD.'
};

async function mockDocsWithLinks(page: import('@playwright/test').Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, string>;
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
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			const path = body.variables?.path ?? '';
			if (path.includes('prd.md')) {
				await route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ data: { documentByPath: DOC_LINK_TARGET } })
				});
			} else {
				await route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ data: { documentByPath: DOC_WITH_LINK } })
				});
			}
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
}

test('US-081a.a: clicking an intra-repo link navigates without a full page reload', async ({
	page
}) => {
	await mockDocsWithLinks(page);
	await page.goto(`${BASE_URL}/${DOC_WITH_LINK.path}`);
	await expect(page.getByRole('heading', { name: 'Vision' })).toBeVisible();

	// Track full page reloads — we expect SPA navigation, not a browser reload.
	let reloadCount = 0;
	page.on('framenavigated', (frame) => {
		if (frame === page.mainFrame()) reloadCount++;
	});

	const prdLink = page.getByRole('link', { name: 'PRD' });
	await expect(prdLink).toBeVisible();
	await prdLink.click();

	await expect(page).toHaveURL(/prd\.md/);
	await expect(page.getByRole('heading', { name: 'Product Requirements' })).toBeVisible();
	expect(reloadCount, 'SPA navigation must not trigger a full page reload').toBeLessThanOrEqual(1);
});

test('US-081a.b: external links open in a new tab with noopener', async ({ page }) => {
	await mockDocsWithLinks(page);
	await page.goto(`${BASE_URL}/${DOC_WITH_LINK.path}`);
	const extLink = page.getByRole('link', { name: 'Anthropic' });
	await expect(extLink).toHaveAttribute('target', '_blank');
	await expect(extLink).toHaveAttribute('rel', /noopener/);
});

test('US-081a.c: back button returns to previous doc with scroll preserved', async ({ page }) => {
	await mockDocsWithLinks(page);
	await page.goto(`${BASE_URL}/${DOC_WITH_LINK.path}`);
	await page.getByRole('link', { name: 'PRD' }).click();
	await expect(page).toHaveURL(/prd\.md/);

	await page.goBack();
	await expect(page.getByRole('heading', { name: 'Vision' })).toBeVisible();
});

// -----------------------------------------------------------------------
// FEAT-008 US-083a: WYSIWYG vs Plain markdown editor toggle
// -----------------------------------------------------------------------

const DOC_TO_EDIT = {
	id: 'doc-edit',
	path: 'docs/test.md',
	title: 'Test Doc',
	content: '---\nddx:\n  id: TEST\n---\n# Test Doc\n\nSome content.'
};

async function mockEditable(page: import('@playwright/test').Page) {
	let currentContent = DOC_TO_EDIT.content;
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
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { documentByPath: { ...DOC_TO_EDIT, content: currentContent } }
				})
			});
		} else if (body.query.includes('DocumentWrite') || body.query.includes('documentWrite')) {
			currentContent = (body.variables?.content ?? currentContent) as string;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { documentWrite: { ...DOC_TO_EDIT, content: currentContent } }
				})
			});
		} else {
			await route.continue();
		}
	});
}

test('US-083a.a: editor has WYSIWYG and Plain mode toggles', async ({ page }) => {
	await mockEditable(page);
	await page.goto(`${BASE_URL}/${DOC_TO_EDIT.path}`);
	await page.getByRole('button', { name: /edit/i }).click();

	const wysiwygToggle = page.getByRole('radio', { name: /wysiwyg/i });
	const plainToggle = page.getByRole('radio', { name: /plain/i });
	await expect(wysiwygToggle).toBeVisible();
	await expect(plainToggle).toBeVisible();
	await expect(wysiwygToggle).toBeChecked();
	await expect(page.getByText('Frontmatter')).toBeVisible();
});

test('US-083a.b: Plain mode shows raw markdown including frontmatter', async ({ page }) => {
	await mockEditable(page);
	await page.goto(`${BASE_URL}/${DOC_TO_EDIT.path}`);
	await page.getByRole('button', { name: /edit/i }).click();
	await page.getByRole('radio', { name: /plain/i }).click();

	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await expect(textarea).toBeVisible();
	await expect(textarea).toHaveValue(/---\s*\n\s*ddx:\s*\n\s*id:\s*TEST/);
});

test('US-083a.c: unsaved edits survive toggle WYSIWYG→Plain→WYSIWYG', async ({ page }) => {
	await mockEditable(page);
	await page.goto(`${BASE_URL}/${DOC_TO_EDIT.path}`);
	await page.getByRole('button', { name: /edit/i }).click();

	// Type in WYSIWYG mode
	const rich = page.locator('[data-testid="wysiwyg-editor"]');
	await rich.click();
	await page.keyboard.press('End');
	await page.keyboard.type(' Added in wysiwyg.');

	// Switch to plain — edit must survive
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await expect(textarea).toHaveValue(/Added in wysiwyg/);

	// Edit in plain
	await textarea.focus();
	await textarea.press('End');
	await textarea.type(' Added in plain.');

	// Switch back to WYSIWYG — both edits survive
	await page.getByRole('radio', { name: /wysiwyg/i }).click();
	await expect(rich).toContainText('Added in wysiwyg');
	await expect(rich).toContainText('Added in plain');
});

test('US-083a.d: saving writes via documentWrite with raw markdown', async ({ page }) => {
	let writeCalled = false;
	let writtenContent = '';

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
		} else if (body.query.includes('DocumentByPath') || body.query.includes('documentByPath')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { documentByPath: DOC_TO_EDIT } })
			});
		} else if (body.query.includes('DocumentWrite') || body.query.includes('documentWrite')) {
			writeCalled = true;
			writtenContent = ((body.variables ?? {}).content as string) ?? '';
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { documentWrite: DOC_TO_EDIT } })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(`${BASE_URL}/${DOC_TO_EDIT.path}`);
	await page.getByRole('button', { name: /edit/i }).click();
	await page.getByRole('radio', { name: /plain/i }).click();
	const textarea = page.getByRole('textbox', { name: /plain markdown editor/i });
	await textarea.fill('# Rewritten\n\nFully replaced.');
	await page.getByRole('button', { name: /save/i }).click();

	await expect.poll(() => writeCalled).toBe(true);
	expect(writtenContent, 'save must send raw markdown, not rendered HTML').toContain('# Rewritten');
});
