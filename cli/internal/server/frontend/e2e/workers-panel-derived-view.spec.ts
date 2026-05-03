import { expect, test } from '@playwright/test';

// ADR-022 step 5b: workers panel migration to the derived-view query.
// Asserts the +layout.svelte renders the new reportedWorkers fields,
// surfaces a freshness badge, shows duplicate-worker entries under one
// project root, and labels reported data as non-authoritative.

let NODE_INFO: { id: string; name: string };
let PROJECT_ID: string;
let PROJECT_PATH: string;
let BASE_URL: string;
let PROJECTS: Array<{ id: string; name: string; path: string }>;

async function getFixtureIds(
	request: import('@playwright/test').APIRequestContext
): Promise<{ nodeId: string; projectId: string; nodeName: string; projectName: string; projectPath: string }> {
	const nodeResp = await request.post('/graphql', {
		data: { query: '{ nodeInfo { id name } }' }
	});
	const nodeBody = (await nodeResp.json()) as {
		data: { nodeInfo: { id: string; name: string } };
	};
	const projectsResp = await request.get('/api/projects');
	const projects = (await projectsResp.json()) as Array<{ id: string; name: string; path: string }>;
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
		nodeName: nodeBody.data.nodeInfo.name,
		projectName: fixture.name,
		projectPath: fixture.path
	};
}

test.beforeEach(async ({ request }) => {
	const ids = await getFixtureIds(request);
	NODE_INFO = { id: ids.nodeId, name: ids.nodeName };
	PROJECT_ID = ids.projectId;
	PROJECT_PATH = ids.projectPath;
	PROJECTS = [{ id: PROJECT_ID, name: ids.projectName, path: ids.projectPath }];
	BASE_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/workers`;
});

interface ReportedWorker {
	id: string;
	project: string;
	harness: string;
	state: string;
	lastEventAt: string;
	mirrorFailuresCount: number;
	hadDroppedBackfill: boolean;
	currentBead: string | null;
	currentAttempt: string | null;
}

function makeWorkersResponse(maxCount: number | null = null) {
	return {
		workersByProject: {
			edges: [],
			pageInfo: { hasNextPage: false, endCursor: null },
			totalCount: 0
		},
		queueAndWorkersSummary: { maxCount }
	};
}

async function mockGraphQL(
	page: import('@playwright/test').Page,
	reportedWorkers: ReportedWorker[]
) {
	// /api/projects is intercepted so the loader resolves the project root path
	// for the current projectId. We respond with the live fixture entry plus a
	// path matching the reportedWorkers fixture so the client-side filter
	// retains the rows.
	await page.route('/api/projects', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify(PROJECTS)
		});
	});

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
		} else if (body.query.includes('ReportedWorkersByProject') || body.query.includes('reportedWorkers')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { reportedWorkers } })
			});
		} else if (body.query.includes('WorkersByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeWorkersResponse() })
			});
		} else if (body.query.includes('QueueAndWorkersSummary')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: { queueAndWorkersSummary: { readyBeads: 0, runningWorkers: 0, totalWorkers: 0, maxCount: null } }
				})
			});
		} else {
			await route.continue();
		}
	});
}

test('reported-workers panel renders derived-view fields and freshness badge', async ({ page }) => {
	const reported: ReportedWorker[] = [
		{
			id: 'reported-w-001',
			project: PROJECT_PATH,
			harness: 'claude',
			state: 'connected',
			lastEventAt: '2026-05-03T07:30:00Z',
			mirrorFailuresCount: 0,
			hadDroppedBackfill: false,
			currentBead: 'bead-derived-001',
			currentAttempt: 'attempt-001'
		}
	];
	await mockGraphQL(page, reported);
	await page.goto(BASE_URL);

	const panel = page.getByTestId('reported-workers-panel');
	await expect(panel).toBeVisible();

	// AC #4: non-authoritative labeling near reported data.
	await expect(panel.getByTestId('reported-not-authoritative')).toBeVisible();
	await expect(panel.getByTestId('reported-not-authoritative')).toHaveText(
		/reported by worker \(not authoritative\)/i
	);

	// AC #1 + #2: freshness state and lastEventAt rendered.
	const freshness = panel.getByTestId('reported-worker-freshness');
	await expect(freshness).toBeVisible();
	await expect(freshness).toHaveAttribute('data-state', 'connected');
	await expect(freshness).toHaveText(/connected/i);
	await expect(panel.getByTestId('reported-worker-last-event')).toBeVisible();

	// AC #1: mirrorFailuresCount + hadDroppedBackfill columns rendered.
	await expect(panel.getByTestId('reported-worker-mirror-failures')).toHaveText('0');
	await expect(panel.getByTestId('reported-worker-dropped-backfill')).toHaveCount(0);

	// currentBead column.
	await expect(panel.getByText('bead-derived-001')).toBeVisible();
});

test('reported-workers panel renders duplicate workers under one project root', async ({ page }) => {
	const reported: ReportedWorker[] = [
		{
			id: 'reported-dup-aaa',
			project: PROJECT_PATH,
			harness: 'claude',
			state: 'connected',
			lastEventAt: '2026-05-03T07:30:00Z',
			mirrorFailuresCount: 0,
			hadDroppedBackfill: false,
			currentBead: 'bead-A',
			currentAttempt: null
		},
		{
			id: 'reported-dup-bbb',
			project: PROJECT_PATH,
			harness: 'codex',
			state: 'stale',
			lastEventAt: '2026-05-03T07:25:00Z',
			mirrorFailuresCount: 2,
			hadDroppedBackfill: true,
			currentBead: 'bead-B',
			currentAttempt: null
		}
	];
	await mockGraphQL(page, reported);
	await page.goto(BASE_URL);

	const panel = page.getByTestId('reported-workers-panel');
	await expect(panel).toBeVisible();

	// AC #3: both workers appear distinctly under the same project root.
	await expect(panel.getByTestId('reported-worker-row')).toHaveCount(2);
	await expect(panel.getByTestId('reported-worker-duplicate-banner')).toHaveText(
		/2 workers reported under this project root/i
	);

	// Both rows visible with distinct ids.
	await expect(panel.locator('[data-worker-id="reported-dup-aaa"]')).toBeVisible();
	await expect(panel.locator('[data-worker-id="reported-dup-bbb"]')).toBeVisible();

	// Stale freshness badge surfaces.
	const freshnessBadges = panel.getByTestId('reported-worker-freshness');
	await expect(freshnessBadges).toHaveCount(2);
	await expect(freshnessBadges.nth(1)).toHaveAttribute('data-state', 'stale');

	// hadDroppedBackfill rendered as a "dropped" badge for the second entry.
	await expect(panel.getByTestId('reported-worker-dropped-backfill')).toHaveCount(1);
	await expect(panel.getByTestId('reported-worker-dropped-backfill')).toHaveText(/dropped/i);

	// mirrorFailuresCount > 0 surfaces non-zero number.
	const mirrorCells = panel.getByTestId('reported-worker-mirror-failures');
	await expect(mirrorCells).toHaveCount(2);
	await expect(mirrorCells.nth(1)).toHaveText('2');
});

test('reported-workers panel shows disconnected freshness state', async ({ page }) => {
	const reported: ReportedWorker[] = [
		{
			id: 'reported-w-disc',
			project: PROJECT_PATH,
			harness: 'claude',
			state: 'disconnected',
			lastEventAt: '2026-05-02T07:00:00Z',
			mirrorFailuresCount: 5,
			hadDroppedBackfill: false,
			currentBead: null,
			currentAttempt: null
		}
	];
	await mockGraphQL(page, reported);
	await page.goto(BASE_URL);

	const panel = page.getByTestId('reported-workers-panel');
	const freshness = panel.getByTestId('reported-worker-freshness');
	await expect(freshness).toHaveAttribute('data-state', 'disconnected');
	await expect(freshness).toHaveText(/disconnected/i);
});

test('reported-workers panel shows empty state when no reports for project', async ({ page }) => {
	await mockGraphQL(page, []);
	await page.goto(BASE_URL);

	await expect(page.getByTestId('reported-workers-empty')).toBeVisible();
	await expect(page.getByTestId('reported-workers-empty')).toHaveText(
		/no reported workers for this project/i
	);
});

test('reported-workers panel filters out workers from other project roots', async ({ page }) => {
	const reported: ReportedWorker[] = [
		{
			id: 'reported-current',
			project: PROJECT_PATH,
			harness: 'claude',
			state: 'connected',
			lastEventAt: '2026-05-03T07:30:00Z',
			mirrorFailuresCount: 0,
			hadDroppedBackfill: false,
			currentBead: null,
			currentAttempt: null
		},
		{
			id: 'reported-other',
			project: '/some/other/project',
			harness: 'codex',
			state: 'connected',
			lastEventAt: '2026-05-03T07:30:00Z',
			mirrorFailuresCount: 0,
			hadDroppedBackfill: false,
			currentBead: null,
			currentAttempt: null
		}
	];
	await mockGraphQL(page, reported);
	await page.goto(BASE_URL);

	const panel = page.getByTestId('reported-workers-panel');
	await expect(panel.locator('[data-worker-id="reported-current"]')).toBeVisible();
	await expect(panel.locator('[data-worker-id="reported-other"]')).toHaveCount(0);
});
