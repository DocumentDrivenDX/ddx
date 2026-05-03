import { expect, test } from '@playwright/test';

let NODE_INFO: { id: string; name: string };
let PROJECT_ID: string;
let PROJECTS: Array<{ id: string; name: string; path: string }>;

const RUN_ID = 'run-try-evidence-001';
const BEAD_ID = 'ddx-evidence-test';

const tryRun = {
	id: RUN_ID,
	layer: 'try',
	status: 'success',
	projectID: '__fixture__',
	beadId: BEAD_ID,
	artifactId: null,
	parentRunId: null,
	childRunIds: [],
	startedAt: '2026-04-30T10:00:00Z',
	completedAt: '2026-04-30T10:30:00Z',
	durationMs: 1_800_000,
	queueInputs: null,
	stopCondition: null,
	selectedBeadIds: null,
	baseRevision: null,
	resultRevision: null,
	worktreePath: null,
	mergeOutcome: null,
	checkResults: null,
	harness: null,
	provider: null,
	model: null,
	promptSummary: null,
	powerMin: null,
	powerMax: null,
	tokensIn: null,
	tokensOut: null,
	costUsd: null,
	outputExcerpt: null,
	evidenceLinks: null,
	bundleFiles: [
		{ path: 'prompt.md', size: 128, mimeType: 'text/markdown' },
		{ path: 'manifest.json', size: 256, mimeType: 'application/json' },
		{ path: 'screenshots/big.png', size: 200_000, mimeType: 'image/png' }
	]
};

async function getFixtureIds(
	request: import('@playwright/test').APIRequestContext
): Promise<{
	nodeId: string;
	projectId: string;
	nodeName: string;
	projectName: string;
	projectPath: string;
}> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as { data: { nodeInfo: { id: string; name: string } } };
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{ id: string; name: string; path: string }>;
	const fixture = projects.find((p) => /(^|\/)ddx-e2e-/.test(p.path) || /^ddx-e2e-/.test(p.name));
	if (!fixture) {
		throw new Error(
			`fixture server has no ddx-e2e-* project registered (got: ${projects.map((p) => p.id).join(', ')})`
		);
	}
	return {
		nodeId: nodeBody.data.nodeInfo.id,
		projectId: fixture.id,
		nodeName: nodeBody.data.nodeInfo.name,
		projectName: fixture.name,
		projectPath: fixture.path
	};
}

test.beforeEach(async ({ request }) => {
	const ids = await getFixtureIds(request);
	NODE_INFO = { id: ids.nodeId, name: ids.nodeName };
	PROJECT_ID = ids.projectId;
	PROJECTS = [{ id: PROJECT_ID, name: ids.projectName, path: ids.projectPath }];
});

test('runs Evidence tab: list bundle files, inline view (whitelist+size), download', async ({
	page
}) => {
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
			return;
		}
		if (body.query.includes('ProjectsForLayout') || body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((node) => ({ node })) } } })
			});
			return;
		}
		if (
			body.query.includes('RunHeader') ||
			body.query.includes('RunDetailExpand') ||
			body.query.includes('RunDetail') ||
			body.query.includes('RunExists')
		) {
			const id = body.variables?.['id'] as string;
			const r = id === RUN_ID ? tryRun : null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r } })
			});
			return;
		}
		if (body.query.includes('ParentRunParent')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: { parentRunId: null } } })
			});
			return;
		}
		if (body.query.includes('RunExecutionExpand')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { execution: null } })
			});
			return;
		}
		if (body.query.includes('RunSessionExpand')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { agentSession: null } })
			});
			return;
		}
		if (body.query.includes('RunToolCallsExpand')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						executionToolCalls: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('RunBundleFileFetch')) {
			const path = body.variables?.['path'] as string;
			let payload: {
				path: string;
				content: string | null;
				sizeBytes: number;
				truncated: boolean;
				mimeType: string;
			};
			if (path === 'prompt.md') {
				payload = {
					path,
					content: '# Sample prompt\nbody text',
					sizeBytes: 128,
					truncated: false,
					mimeType: 'text/markdown'
				};
			} else if (path === 'screenshots/big.png') {
				payload = {
					path,
					content: null,
					sizeBytes: 200_000,
					truncated: true,
					mimeType: 'image/png'
				};
			} else {
				payload = {
					path,
					content: '{"ok":true}',
					sizeBytes: 256,
					truncated: false,
					mimeType: 'application/json'
				};
			}
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { runBundleFile: payload } })
			});
			return;
		}
		if (body.query.includes('ProducedArtifact')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { artifact: null } })
			});
			return;
		}
		await route.continue();
	});

	// Navigate directly to evidence tab
	await page.goto(
		`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}?tab=evidence`
	);
	await expect(page.locator('h1', { hasText: RUN_ID })).toBeVisible();

	const detail = page.locator('[data-testid="rundetail"]');
	await expect(detail.locator('button[data-tab="evidence"]')).toBeVisible();
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute(
		'data-active-tab',
		'evidence'
	);

	// AC1: list shows the three bundle files
	const evidence = page.locator('[data-testid="rundetail-evidence"]');
	await expect(evidence).toBeVisible();
	await expect(evidence.locator('[data-evidence-path="prompt.md"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-path="manifest.json"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-path="screenshots/big.png"]')).toBeVisible();

	// AC2: View whitelisted small text -> inline content visible
	await evidence.locator('[data-evidence-view="prompt.md"]').click();
	await expect(evidence.locator('[data-testid="evidence-inline-content"]')).toContainText(
		'Sample prompt'
	);

	// AC2: View non-whitelisted (or oversize) -> truncated message, not content
	await evidence.locator('[data-evidence-view="screenshots/big.png"]').click();
	await expect(evidence.locator('[data-testid="evidence-inline-truncated"]')).toBeVisible();

	// AC3: Download link points at the bundle download endpoint
	const dl = evidence.locator('[data-evidence-download="prompt.md"]');
	await expect(dl).toBeVisible();
	await expect(dl).toHaveAttribute(
		'href',
		`/api/runs/${encodeURIComponent(RUN_ID)}/bundle?path=${encodeURIComponent('prompt.md')}`
	);
	await expect(dl).toHaveAttribute('download', 'prompt.md');
});
