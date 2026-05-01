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
