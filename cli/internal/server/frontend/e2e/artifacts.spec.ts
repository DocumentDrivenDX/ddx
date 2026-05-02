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
	generatedBy: null
};

const REGENERATE_RUN_ID = 'regen-artifact-generated-001-deadbeef';

const BASE_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/artifacts`;

interface MockState {
	regenerateCalled: boolean;
	lastArtifactId: string | null;
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

// Full US-081b workflow: list → filter → open → renderer → provenance →
// Regenerate → run id shown → follow run link → navigate back to artifact.
test('US-081b end-to-end: list → filter → open → regenerate → run link → back', async ({
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
	await page.getByText('Generated Report').first().click();
	await expect(page).toHaveURL(new RegExp(`/artifacts/${ARTIFACT_GENERATED.id}`));

	// 4. Verify renderer (markdown body shown). The page renders the artifact
	//    title as <h1> and the markdown body also renders <h1># Generated
	//    Report</h1>, so we use .first() to disambiguate without weakening the
	//    visibility assertion.
	await expect(page.getByRole('heading', { name: 'Generated Report' }).first()).toBeVisible();
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
test('Regenerate button is not shown when generatedBy is absent', async ({ page }) => {
	const state: MockState = { regenerateCalled: false, lastArtifactId: null };
	await mockRoutes(page, state);

	await page.goto(`${BASE_URL}/${ARTIFACT_PLAIN.id}`);
	await expect(page.getByRole('heading', { name: 'Manual Doc' })).toBeVisible();
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

// Mutation error path: server returns a typed error → inline message in the
// provenance panel, page does not crash (AC#4).
test('Regenerate error renders inline without crashing the page', async ({ page }) => {
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

	await page.goto(`${BASE_URL}/${ARTIFACT_GENERATED.id}`);
	await page.getByTestId('regenerate-button').click();
	await expect(page.getByTestId('regenerate-error')).toContainText(
		/regeneration backend unavailable/
	);
	// Page is still alive.
	await expect(page.getByRole('heading', { name: 'Generated Report' }).first()).toBeVisible();
});
