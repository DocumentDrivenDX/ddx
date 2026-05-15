import { expect, test } from '@playwright/test';

// Seeded fixture: an artifact with known generated_by metadata (US-081b).
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

const ARTIFACT_GENERATED = {
	id: 'artifact-generated-001',
	path: 'docs/generated/report.md',
	title: 'Generated Report',
	mediaType: 'text/markdown',
	staleness: 'fresh',
	description: 'Synthesized report',
	updatedAt: '2026-04-30T12:00:00Z',
	ddxFrontmatter: JSON.stringify({ id: 'artifact-generated-001' }),
	content: '# Generated Report\n\nBody text.\n',
	typeDefinitions: [],
	generatedBy: {
		runId: 'run-source-123',
		promptSummary: 'synthesize report from latest data',
		sourceHashMatch: true
	}
};

const ARTIFACT_PLAIN = {
	id: 'artifact-plain-001',
	path: 'docs/manual.md',
	title: 'Manual Doc',
	mediaType: 'text/markdown',
	staleness: 'fresh',
	description: null,
	updatedAt: '2026-04-30T12:00:00Z',
	ddxFrontmatter: null,
	content: '# Manual\n',
	typeDefinitions: [],
	generatedBy: null
};

const REGENERATE_RUN_ID = 'regen-artifact-generated-001-deadbeef';

const BASE_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/artifacts`;

interface MockState {
	regenerateCalled: boolean;
	lastArtifactId: string | null;
}

interface MockArtifactTypeFile {
	path: string;
	content: string;
	isTruncated: boolean;
	sizeBytes: number;
}

interface MockArtifactTypeExample extends MockArtifactTypeFile {
	description?: string | null;
}

interface MockArtifactTypeDefinition {
	plugin: string;
	typeId: string;
	name: string;
	description: string;
	prefix: string;
	pattern: string;
	phase: string;
	sourceMetaPath: string;
	template: MockArtifactTypeFile;
	prompt: MockArtifactTypeFile;
	examples: MockArtifactTypeExample[];
}

interface MockArtifactDetail {
	id: string;
	path: string;
	title: string;
	mediaType: string;
	staleness: string;
	description: string | null;
	updatedAt: string | null;
	ddxFrontmatter: string | null;
	content: string | null;
	typeDefinitions: MockArtifactTypeDefinition[];
	generatedBy: {
		runId: string;
		promptSummary: string;
		sourceHashMatch: boolean;
	} | null;
}

function typeFile(
	path: string,
	content: string,
	overrides?: Partial<MockArtifactTypeFile>
): MockArtifactTypeFile {
	return {
		path,
		content,
		isTruncated: false,
		sizeBytes: content.length,
		...overrides
	};
}

function typeDefinition(
	overrides?: Partial<MockArtifactTypeDefinition>
): MockArtifactTypeDefinition {
	return {
		plugin: 'ddx',
		typeId: 'docs-sample',
		name: 'Docs sample',
		description: 'Sample artifact type definition',
		prefix: 'docs',
		pattern: 'docs/*.md',
		phase: 'frame',
		sourceMetaPath: 'plugins/ddx/workflows/phases/01-frame/artifacts/docs-sample/meta.yml',
		template: typeFile('template.md', '# template one'),
		prompt: typeFile('prompt.md', '# prompt one'),
		examples: [],
		...overrides
	};
}

const ARTIFACT_TYPE_SINGLE: MockArtifactDetail = {
	id: 'artifact-type-single-001',
	path: 'docs/helix/01-frame/single.md',
	title: 'Artifact Type Single',
	mediaType: 'text/markdown',
	staleness: 'fresh',
	description: 'Artifact detail fixture with one matching type definition',
	updatedAt: '2026-05-01T12:00:00Z',
	ddxFrontmatter: JSON.stringify({ id: 'artifact-type-single-001' }),
	content: '# Single artifact\n',
	typeDefinitions: [
		typeDefinition({
			description: 'Single definition rendered without a collision selector',
			template: typeFile('template.md', '# template one\n\nBody.\n'),
			prompt: typeFile('prompt.md', '# prompt one\n\nReference prompt.\n'),
			examples: [
				{
					path: 'example.md',
					description: 'Worked example',
					content: '# example\n\nUseful example content.\n',
					isTruncated: false,
					sizeBytes: 33
				}
			]
		})
	],
	generatedBy: null
};

const ARTIFACT_TYPE_COLLISION: MockArtifactDetail = {
	id: 'artifact-type-collision-001',
	path: 'docs/helix/01-frame/collision.md',
	title: 'Artifact Type Collision',
	mediaType: 'text/markdown',
	staleness: 'fresh',
	description: 'Artifact detail fixture with two matching type definitions',
	updatedAt: '2026-05-01T12:00:00Z',
	ddxFrontmatter: JSON.stringify({ id: 'artifact-type-collision-001' }),
	content: '# Collision artifact\n',
	typeDefinitions: [
		typeDefinition({
			plugin: 'alpha',
			typeId: 'docs-alpha',
			name: 'Alpha docs',
			description: 'First matching definition',
			sourceMetaPath: 'plugins/alpha/workflows/phases/01-frame/artifacts/docs-alpha/meta.yml',
			template: typeFile('alpha-template.md', '# alpha template\n'),
			prompt: typeFile('alpha-prompt.md', '# alpha prompt\n')
		}),
		typeDefinition({
			plugin: 'beta',
			typeId: 'docs-beta',
			name: 'Beta docs',
			description: 'Second matching definition',
			sourceMetaPath: 'plugins/beta/workflows/phases/01-frame/artifacts/docs-beta/meta.yml',
			template: typeFile('beta-template.md', '# beta template\n'),
			prompt: typeFile('beta-prompt.md', '# beta prompt\n')
		})
	],
	generatedBy: null
};

const TRUNCATED_TEMPLATE_CONTENT = [
	'# large template preview',
	'',
	'## Section A',
	'This is a stable truncated preview for the artifact type template panel.',
	'',
	'## Section B',
	'- keep the path label visible',
	'- keep the truncated badge visible',
	'- keep the inline content preview visible',
	'',
	'## Section C',
	'The underlying source file is larger than 64KB, but the UI only renders',
	'the truncated preview returned by the GraphQL resolver.'
].join('\n');

const ARTIFACT_TYPE_TRUNCATED: MockArtifactDetail = {
	id: 'artifact-type-truncated-001',
	path: 'docs/helix/01-frame/large.md',
	title: 'Artifact Type Truncated',
	mediaType: 'text/markdown',
	staleness: 'fresh',
	description: 'Artifact detail fixture with a truncated template payload',
	updatedAt: '2026-05-01T12:00:00Z',
	ddxFrontmatter: JSON.stringify({ id: 'artifact-type-truncated-001' }),
	content: '# Truncated artifact\n',
	typeDefinitions: [
		typeDefinition({
			typeId: 'docs-large',
			name: 'Docs large',
			description: 'Large template fixture for screenshot coverage',
			sourceMetaPath: 'plugins/ddx/workflows/phases/01-frame/artifacts/docs-large/meta.yml',
			template: typeFile('template.md', TRUNCATED_TEMPLATE_CONTENT, {
				isTruncated: true,
				sizeBytes: 70 * 1024
			}),
			prompt: typeFile('prompt.md', '# small prompt\n')
		})
	],
	generatedBy: null
};

function artifactDetailHref(artifactID: string): string {
	return `${BASE_URL}/${encodeURIComponent(artifactID)}`;
}

async function mockArtifactTypeRoutes(
	page: import('@playwright/test').Page,
	artifacts: MockArtifactDetail[]
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, string>;
		};
		const q = body.query;

		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('Projects') && !q.includes('projectID')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
			return;
		}
		if (q.includes('ArtifactDetail')) {
			const artifact = artifacts.find((item) => item.id === body.variables?.id) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { artifact } })
			});
			return;
		}
		if (q.includes('RunExists')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: null } })
			});
			return;
		}
		await route.continue();
	});
}

async function mockRoutes(page: import('@playwright/test').Page, state: MockState) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, string>;
		};
		const q = body.query;

		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('Projects') && !q.includes('projectID')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
			return;
		}
		if (q.includes('query Artifacts(') || q.includes('ArtifactsByPath')) {
			const list = [ARTIFACT_GENERATED, ARTIFACT_PLAIN];
			const search = body.variables?.search;
			const filtered = search
				? list.filter(
						(a) =>
							a.title.toLowerCase().includes(String(search).toLowerCase()) ||
							a.path.toLowerCase().includes(String(search).toLowerCase())
					)
				: list;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifacts: {
							edges: filtered.map((a, i) => ({
								node: {
									id: a.id,
									path: a.path,
									title: a.title,
									mediaType: a.mediaType,
									staleness: a.staleness
								},
								cursor: `c${i}`
							})),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: filtered.length
						}
					}
				})
			});
			return;
		}
		if (q.includes('ArtifactDetail')) {
			const id = body.variables?.id;
			const match = [ARTIFACT_GENERATED, ARTIFACT_PLAIN].find((a) => a.id === id);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { artifact: match ?? null } })
			});
			return;
		}
		if (q.includes('RunExists')) {
			const id = body.variables?.id;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { run: id === ARTIFACT_GENERATED.generatedBy.runId ? { id } : null }
				})
			});
			return;
		}
		if (q.includes('mutation ArtifactRegenerate')) {
			state.regenerateCalled = true;
			state.lastArtifactId = body.variables?.artifactId ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifactRegenerate: { runId: REGENERATE_RUN_ID, status: 'queued' }
					}
				})
			});
			return;
		}
		if (q.includes('Run(') || q.includes('query Run')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: { id: REGENERATE_RUN_ID } } })
			});
			return;
		}
		await route.continue();
	});
}

test('artifact type panel: single match renders without a collision selector', async ({
	page
}) => {
	await mockArtifactTypeRoutes(page, [ARTIFACT_TYPE_SINGLE]);

	await page.goto(artifactDetailHref(ARTIFACT_TYPE_SINGLE.id), { waitUntil: 'networkidle' });

	const panel = page.getByTestId('artifact-type-panel');
	await expect(panel).toBeVisible();
	await expect(panel).toContainText('Artifact type');
	await expect(panel).toContainText('Prefix docs');
	await expect(panel).toContainText('Docs sample');
	await expect(panel).toContainText(/ddx\/\s*docs-sample/);
	await expect(page.getByTestId('artifact-type-selector')).toHaveCount(0);

	await expect(page.getByTestId('artifact-type-reference-prompt')).toContainText('# prompt one');
	await page.getByTestId('artifact-type-tab-template').click();
	await expect(page.getByTestId('artifact-type-template')).toContainText('# template one');
	await page.getByTestId('artifact-type-tab-examples').click();
	await expect(page.getByTestId('artifact-type-examples')).toContainText('Worked example');
	await expect(page.getByTestId('artifact-type-examples')).toContainText('Useful example content.');
});

test('artifact type panel: collision selector switches definitions and survives refresh', async ({
	page
}) => {
	await mockArtifactTypeRoutes(page, [ARTIFACT_TYPE_COLLISION]);

	await page.goto(artifactDetailHref(ARTIFACT_TYPE_COLLISION.id), { waitUntil: 'networkidle' });

	const selector = page.getByTestId('artifact-type-selector');
	const betaKey =
		'beta::docs-beta::plugins/beta/workflows/phases/01-frame/artifacts/docs-beta/meta.yml';

	await expect(selector).toBeVisible();
	await expect(page.getByTestId('artifact-type-reference-prompt')).toContainText('# alpha prompt');

	await selector.selectOption(betaKey);
	await expect(page).toHaveURL(/typeDef=beta%3A%3Adocs-beta%3A%3Aplugins%2Fbeta/);
	await expect(page.getByTestId('artifact-type-reference-prompt')).toContainText('# beta prompt');

	await page.reload({ waitUntil: 'networkidle' });
	await expect(selector).toHaveValue(betaKey);
	await expect(page.getByTestId('artifact-type-reference-prompt')).toContainText('# beta prompt');
});

test('artifact type panel: truncated template snapshot is stable', async ({ page }) => {
	await mockArtifactTypeRoutes(page, [ARTIFACT_TYPE_TRUNCATED]);
	await page.setViewportSize({ width: 1100, height: 900 });

	await page.goto(artifactDetailHref(ARTIFACT_TYPE_TRUNCATED.id), { waitUntil: 'networkidle' });
	await page.getByTestId('artifact-type-tab-template').click();

	const templatePanel = page.getByTestId('artifact-type-template');
	await expect(templatePanel).toBeVisible();
	await expect(templatePanel).toContainText('Truncated');
	await expect(templatePanel).toHaveScreenshot('artifact-type-template-truncated.png', {
		animations: 'disabled',
		caret: 'hide'
	});
});

// Full US-081b workflow: list → filter → open → renderer → provenance →
// Regenerate → run id shown → follow run link → navigate back to artifact.
test.fixme('US-081b end-to-end: list → filter → open → regenerate → run link → back', async ({
	page
}) => {
	const state: MockState = { regenerateCalled: false, lastArtifactId: null };
	await mockRoutes(page, state);

	// 1. List
	await page.goto(BASE_URL);
	await expect(page.getByText('Generated Report')).toBeVisible();
	await expect(page.getByText('Manual Doc')).toBeVisible();

	// 2. Filter (search) — narrow to the generated artifact via path/title.
	const search = page.getByPlaceholder(/Search/i).first();
	await search.fill('Generated');
	await expect(page.getByText('Generated Report')).toBeVisible();

	// 3. Open
	await page.goto(`${BASE_URL}/${encodeURIComponent(ARTIFACT_GENERATED.id)}`, {
		waitUntil: 'networkidle'
	});
	await expect(page).toHaveURL(new RegExp(`/artifacts/${ARTIFACT_GENERATED.id}`));

	// 4. Verify renderer (markdown body shown). The page renders the artifact
	//    title is rendered in the detail view, and the markdown body also renders
	//    the same text. Use exact text assertions rather than role lookup to keep
	//    the check resilient to heading semantics.
	await expect(page.getByText('Generated Report', { exact: true })).toBeVisible();
	await expect(page.getByText(/Body text\./)).toBeVisible();

	// 5. Provenance panel visible.
	await expect(page.getByTestId('provenance-panel')).toBeVisible();

	// 6. Click Regenerate.
	await page.getByTestId('regenerate-button').click();

	// 7. Verify run id shown + dispatched mutation called.
	const runLink = page.getByTestId('regenerate-run-link');
	await expect(runLink).toBeVisible();
	await expect(runLink).toHaveText(REGENERATE_RUN_ID);
	expect(state.regenerateCalled).toBe(true);
	expect(state.lastArtifactId).toBe(ARTIFACT_GENERATED.id);

	// 8. Follow run link.
	await runLink.click();
	await expect(page).toHaveURL(
		new RegExp(`/runs/${encodeURIComponent(REGENERATE_RUN_ID).replace(/-/g, '\\-')}`)
	);

	// 9. Navigate back to artifact.
	await page.goBack();
	await expect(page).toHaveURL(new RegExp(`/artifacts/${ARTIFACT_GENERATED.id}`));
	await expect(page.getByTestId('provenance-panel')).toBeVisible();
});

// Regenerate button is hidden when generatedBy is absent (AC#2).
test.fixme('Regenerate button is not shown when generatedBy is absent', async ({ page }) => {
	const state: MockState = { regenerateCalled: false, lastArtifactId: null };
	await mockRoutes(page, state);

	await page.goto(BASE_URL);
	await page.goto(`${BASE_URL}/${encodeURIComponent(ARTIFACT_PLAIN.id)}`, {
		waitUntil: 'networkidle'
	});
	await expect(page.getByText('Manual Doc', { exact: true })).toBeVisible();
	await expect(page.getByTestId('provenance-panel')).toHaveCount(0);
	await expect(page.getByTestId('regenerate-button')).toHaveCount(0);
});

// Sort dropdown + staleness chips + search composition (Story 6).
// Verifies: (1) controls render, (2) URL state round-trips, (3) each param
// change re-issues the query (cursor reset) with the merged variables.
test('artifacts: sort + staleness chips + search compose into URL state and refetch', async ({
	page
}) => {
	const calls: { variables: Record<string, unknown> | undefined; after: unknown }[] = []
	const listArtifacts = [
		{
			id: 'a-1',
			path: 'docs/alpha.md',
			title: 'Alpha',
			mediaType: 'text/markdown',
			staleness: 'fresh'
		},
		{
			id: 'a-2',
			path: 'docs/beta.md',
			title: 'Beta',
			mediaType: 'text/markdown',
			staleness: 'stale'
		},
		{
			id: 'a-3',
			path: 'docs/gamma.md',
			title: 'Gamma',
			mediaType: 'text/markdown',
			staleness: 'missing'
		}
	]
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string
			variables?: Record<string, unknown>
		}
		const q = body.query
		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			})
			return
		}
		if (q.includes('Projects') && !q.includes('projectID')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			})
			return
		}
		if (q.includes('query Artifacts(')) {
			calls.push({ variables: body.variables, after: body.variables?.after ?? null })
			const v = body.variables ?? {}
			let filtered = listArtifacts
			if (v.staleness) filtered = filtered.filter((a) => a.staleness === v.staleness)
			if (v.search) {
				const s = String(v.search).toLowerCase()
				filtered = filtered.filter(
					(a) => a.title.toLowerCase().includes(s) || a.path.toLowerCase().includes(s)
				)
			}
			if (v.sort === 'TITLE') {
				filtered = [...filtered].sort((a, b) => a.title.localeCompare(b.title))
			}
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifacts: {
							edges: filtered.map((a, i) => ({
								node: a,
								cursor: `c${i}`
							})),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: filtered.length
						}
					}
				})
			})
			return
		}
		await route.continue()
	})

	// 1. Initial load — sort dropdown + staleness chips visible.
	await page.goto(BASE_URL)
	await expect(page.getByTestId('sort-select')).toBeVisible()
	await expect(page.getByTestId('staleness-chip-fresh')).toBeVisible()
	await expect(page.getByTestId('staleness-chip-stale')).toBeVisible()
	await expect(page.getByTestId('staleness-chip-missing')).toBeVisible()
	await expect(page.getByRole('cell', { name: 'Alpha', exact: true })).toBeVisible()

	// 2. Change sort → URL gains ?sort=TITLE and a refetch is issued.
	const callsBefore = calls.length
	await page.getByTestId('sort-select').selectOption('TITLE')
	await expect(page).toHaveURL(/[?&]sort=TITLE\b/)
	await expect.poll(() => calls.length).toBeGreaterThan(callsBefore)
	expect(calls[calls.length - 1].variables?.sort).toBe('TITLE')
	expect(calls[calls.length - 1].after).toBeNull()

	// 3. Toggle staleness=stale chip → URL gains ?staleness=stale and refetches
	//    (cursor reset: `after` is null on the new request).
	const beforeStale = calls.length
	await page.getByTestId('staleness-chip-stale').click()
	await expect(page).toHaveURL(/[?&]staleness=stale\b/)
	await expect.poll(() => calls.length).toBeGreaterThan(beforeStale)
	expect(calls[calls.length - 1].variables?.staleness).toBe('stale')
	expect(calls[calls.length - 1].variables?.sort).toBe('TITLE')
	expect(calls[calls.length - 1].after).toBeNull()
	await expect(page.getByRole('cell', { name: 'Beta', exact: true })).toBeVisible()
	await expect(page.getByRole('cell', { name: 'Alpha', exact: true })).toHaveCount(0)

	// 4. Add search → params compose; URL contains all three; refetch issued.
	const beforeSearch = calls.length
	await page.getByPlaceholder(/Search/i).first().fill('beta')
	await expect.poll(() => calls.length).toBeGreaterThan(beforeSearch)
	await expect(page).toHaveURL(/[?&]q=beta\b/)
	await expect(page).toHaveURL(/[?&]sort=TITLE\b/)
	await expect(page).toHaveURL(/[?&]staleness=stale\b/)
	const last = calls[calls.length - 1]
	expect(last.variables?.search).toBe('beta')
	expect(last.variables?.sort).toBe('TITLE')
	expect(last.variables?.staleness).toBe('stale')
	expect(last.after).toBeNull()

	// 5. Toggle staleness chip off → param removed; refetch reflects null.
	const beforeClear = calls.length
	await page.getByTestId('staleness-chip-stale').click()
	await expect.poll(() => calls.length).toBeGreaterThan(beforeClear)
	await expect(page).not.toHaveURL(/[?&]staleness=/)
	expect(calls[calls.length - 1].variables?.staleness).toBeUndefined()
})

// Full-text body match path (Story 6 B4c): server returns a snippet wrapped
// in markdown emphasis markers; the list renders it with the matched span
// highlighted, and back-navigation from the detail page preserves the
// filter+search URL state and the snippet is shown again on return.
test('artifacts: body-match snippet renders with highlight and back-nav preserves state', async ({
	page
}) => {
	const ARTIFACT_BODY = {
		id: 'artifact-body-001',
		path: 'docs/notes.md',
		title: 'Architecture Notes',
		mediaType: 'text/markdown',
		staleness: 'fresh',
		description: null,
		updatedAt: '2026-04-30T12:00:00Z',
		ddxFrontmatter: null,
		content: '# Notes\n\nThe quick brown fox jumps over the lazy dog.\n',
		typeDefinitions: [],
		generatedBy: null
	};
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		const q = body.query;
		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('Projects') && !q.includes('projectID')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
			return;
		}
		if (q.includes('query Artifacts(')) {
			const search = body.variables?.search ? String(body.variables.search) : '';
			// Body-match path: title/path don't match "fox", but the body does;
			// the resolver returns a windowed snippet with **fox** marker.
			const matches = search.toLowerCase() === 'fox';
			const edges = matches
				? [
						{
							node: {
								id: ARTIFACT_BODY.id,
								path: ARTIFACT_BODY.path,
								title: ARTIFACT_BODY.title,
								mediaType: ARTIFACT_BODY.mediaType,
								staleness: ARTIFACT_BODY.staleness
							},
							cursor: 'c0',
							snippet: '…The quick brown **fox** jumps over the lazy dog.'
						}
					]
				: [
						{
							node: {
								id: ARTIFACT_BODY.id,
								path: ARTIFACT_BODY.path,
								title: ARTIFACT_BODY.title,
								mediaType: ARTIFACT_BODY.mediaType,
								staleness: ARTIFACT_BODY.staleness
							},
							cursor: 'c0',
							snippet: null
						}
					];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifacts: {
							edges,
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: edges.length
						}
					}
				})
			});
			return;
		}
		if (q.includes('ArtifactDetail')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { artifact: ARTIFACT_BODY } })
			});
			return;
		}
		await route.continue();
	});

	// 1. Initial load shows the artifact without a snippet (no search).
	await page.goto(BASE_URL);
	await expect(page.getByRole('cell', { name: 'Architecture Notes' })).toBeVisible();
	await expect(page.getByTestId(`artifact-snippet-${ARTIFACT_BODY.id}`)).toHaveCount(0);

	// 2. Search "fox" — body-match returns snippet with **fox** marker.
	await page.getByPlaceholder(/Search/i).first().fill('fox');
	await expect(page).toHaveURL(/[?&]q=fox\b/);
	const snippetRow = page.getByTestId(`artifact-snippet-${ARTIFACT_BODY.id}`);
	await expect(snippetRow).toBeVisible();
	// Match is wrapped in <mark>; surrounding text rendered as plain text.
	await expect(snippetRow.locator('mark')).toHaveText('fox');
	await expect(snippetRow).toContainText('The quick brown');
	await expect(snippetRow).toContainText('jumps over the lazy dog');

	// 3. Open detail — the list URL (with q=fox) is passed via ?back=…
	await page.getByRole('cell', { name: 'Architecture Notes' }).click();
	await expect(page).toHaveURL(new RegExp(`/artifacts/${ARTIFACT_BODY.id}`));
	await expect(page).toHaveURL(/[?&]back=/);

	// 4. goBack() returns to the filtered list with q=fox preserved and the
	//    snippet displayed again (verifies URL state survived round-trip).
	await page.goBack();
	await expect(page).toHaveURL(/[?&]q=fox\b/);
	await expect(snippetRow).toBeVisible();
	await expect(snippetRow.locator('mark')).toHaveText('fox');
});

// Group-by axes (folder/prefix/media type/workflow stage) plus search
// composition. Verifies the page can switch among all grouping modes, then
// keep the active grouping while a search query narrows the visible subset.
test('artifacts: grouping axes render and compose with search filtering', async ({ page }) => {
	const GROUPED_ARTIFACTS = [
		{
			id: 'artifact-frame-001',
			path: 'docs/helix/01-frame/brief.md',
			title: 'Frame Brief',
			mediaType: 'text/markdown',
			staleness: 'fresh'
		},
		{
			id: 'artifact-design-001',
			path: 'docs/helix/02-design/spec.md',
			title: 'Design Spec',
			mediaType: 'text/markdown',
			staleness: 'stale'
		},
		{
			id: 'artifact-readme-001',
			path: 'docs/README.md',
			title: 'Docs Readme',
			mediaType: 'text/markdown',
			staleness: 'fresh'
		},
		{
			id: 'artifact-logo-001',
			path: 'src/assets/logo.svg',
			title: 'Logo',
			mediaType: 'image/svg+xml',
			staleness: 'fresh'
		},
		{
			id: 'artifact-report-001',
			path: 'src/reports/report.pdf',
			title: 'Report',
			mediaType: 'application/pdf',
			staleness: 'missing'
		}
	];

	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		const q = body.query;
		if (q.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (q.includes('Projects') && !q.includes('projectID')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } }
				})
			});
			return;
		}
		if (q.includes('query Artifacts(') || q.includes('ArtifactsByPath')) {
			const search = String(body.variables?.search ?? '').toLowerCase();
			const filtered = search
				? GROUPED_ARTIFACTS.filter(
						(a) =>
							a.title.toLowerCase().includes(search) || a.path.toLowerCase().includes(search)
					)
				: GROUPED_ARTIFACTS;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						artifacts: {
							edges: filtered.map((a, i) => ({
								node: {
									id: a.id,
									path: a.path,
									title: a.title,
									mediaType: a.mediaType,
									staleness: a.staleness
								},
								cursor: `c${i}`
							})),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: filtered.length
						}
					}
				})
			});
			return;
		}
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ data: {} })
		});
	});

	await page.goto(BASE_URL);
	await expect(page.getByRole('rowgroup', { name: 'Folder: docs', exact: true })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Folder: docs/helix/01-frame' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Folder: docs/helix/02-design' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Folder: src/assets' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Folder: src/reports' })).toBeVisible();

	await page.getByLabel('Group by').selectOption('prefix');
	await expect(page).toHaveURL(/[?&]groupBy=prefix\b/);
	await expect(page.getByRole('rowgroup', { name: 'Prefix: docs' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Prefix: src' })).toBeVisible();

	await page.getByLabel('Group by').selectOption('mediaType');
	await expect(page).toHaveURL(/[?&]groupBy=mediaType\b/);
	await expect(page.getByRole('rowgroup', { name: 'Media type: application/pdf' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Media type: image/svg+xml' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Media type: text/markdown' })).toBeVisible();

	await page.getByLabel('Group by').selectOption('workflowStage');
	await expect(page).toHaveURL(/[?&]groupBy=workflowStage\b/);
	await expect(page.getByRole('rowgroup', { name: 'Workflow stage: design' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Workflow stage: frame' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Workflow stage: Unstaged' })).toBeVisible();

	await page.getByPlaceholder(/Search/i).first().fill('docs');
	await expect(page).toHaveURL(/[?&]q=docs\b/);
	await expect(page).toHaveURL(/[?&]groupBy=workflowStage\b/);
	await expect(page.getByRole('rowgroup', { name: 'Workflow stage: design' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Workflow stage: frame' })).toBeVisible();
	await expect(page.getByRole('rowgroup', { name: 'Workflow stage: Unstaged' })).toBeVisible();
	await expect(page.getByRole('cell', { name: 'Logo', exact: true })).toHaveCount(0);
	await expect(page.getByRole('cell', { name: 'Report', exact: true })).toHaveCount(0);
	await expect(page.getByRole('cell', { name: 'Docs Readme', exact: true })).toBeVisible();
});

// Mutation error path: server returns a typed error → inline message in the
// provenance panel, page does not crash (AC#4).
test.fixme('Regenerate error renders inline without crashing the page', async ({ page }) => {
	const state: MockState = { regenerateCalled: false, lastArtifactId: null };
	await mockRoutes(page, state);
	// Override only the mutation to return a typed error.
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string };
		if (body.query.includes('mutation ArtifactRegenerate')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: null,
					errors: [
						{
							message: 'regeneration backend unavailable',
							extensions: { code: 'INTERNAL_ERROR' }
						}
					]
				})
			});
			return;
		}
		await route.fallback();
	});

	await page.goto(BASE_URL);
	await page.goto(`${BASE_URL}/${encodeURIComponent(ARTIFACT_GENERATED.id)}`, {
		waitUntil: 'networkidle'
	});
	await expect(page.getByText('Generated Report', { exact: true })).toBeVisible();
	await page.getByTestId('regenerate-button').click();
	await expect(page.getByTestId('regenerate-error')).toContainText(
		/regeneration backend unavailable/
	);
	// Page is still alive.
	await expect(page.getByText('Generated Report', { exact: true })).toBeVisible();
});
