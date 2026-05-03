import { expect, test } from '@playwright/test';

let NODE_INFO: { id: string; name: string };
let PROJECT_ID: string;
let PROJECTS: Array<{ id: string; name: string; path: string }>;

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

const WORK_ID = 'run-work-001';
const TRY_ID = 'run-try-001';
const RUN_ID = 'run-run-001';
const BEAD_ID = 'ddx-runs-test';
const ARTIFACT_ID = 'art-runs-001';

const workNode = {
	id: WORK_ID,
	layer: 'work',
	status: 'success',
	projectID: '__fixture__',
	beadId: null,
	artifactId: null,
	parentRunId: null,
	childRunIds: [TRY_ID],
	startedAt: '2026-04-30T10:00:00Z',
	completedAt: '2026-04-30T10:30:00Z',
	durationMs: 1_800_000,
	queueInputs: '{"selected":["ddx-runs-test"]}',
	stopCondition: 'queue-empty',
	selectedBeadIds: [BEAD_ID],
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
	evidenceLinks: null
};

const tryNode = {
	id: TRY_ID,
	layer: 'try',
	status: 'success',
	projectID: '__fixture__',
	beadId: BEAD_ID,
	artifactId: null,
	parentRunId: WORK_ID,
	childRunIds: [RUN_ID],
	startedAt: '2026-04-30T10:05:00Z',
	completedAt: '2026-04-30T10:20:00Z',
	durationMs: 900_000,
	queueInputs: null,
	stopCondition: null,
	selectedBeadIds: null,
	baseRevision: 'abc123def',
	resultRevision: 'def456abc',
	worktreePath: '/tmp/ddx-exec-wt/.try-001',
	mergeOutcome: 'merged',
	checkResults: '{"all":"ok"}',
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
	evidenceLinks: null
};

const runNode = {
	id: RUN_ID,
	layer: 'run',
	status: 'success',
	projectID: '__fixture__',
	beadId: BEAD_ID,
	artifactId: ARTIFACT_ID,
	parentRunId: TRY_ID,
	childRunIds: [],
	startedAt: '2026-04-30T10:06:00Z',
	completedAt: '2026-04-30T10:18:00Z',
	durationMs: 720_000,
	queueInputs: null,
	stopCondition: null,
	selectedBeadIds: null,
	baseRevision: null,
	resultRevision: null,
	worktreePath: null,
	mergeOutcome: null,
	checkResults: null,
	harness: 'claude',
	provider: 'anthropic',
	model: 'claude-sonnet-4-6',
	promptSummary: 'execute bead test',
	powerMin: 2,
	powerMax: 4,
	tokensIn: 12000,
	tokensOut: 3400,
	costUsd: 0.0876,
	outputExcerpt: 'completed with success',
	evidenceLinks: ['.ddx/executions/20260430T100600/evidence.txt']
};

const ALL_RUNS = [workNode, tryNode, runNode];

function listRowFor(node: typeof workNode) {
	return {
		node: {
			id: node.id,
			layer: node.layer,
			status: node.status,
			projectID: PROJECT_ID,
			beadId: node.beadId,
			startedAt: node.startedAt,
			durationMs: node.durationMs,
			harness: node.harness
		},
		cursor: node.id
	};
}

test('runs work→try→run drill-down with breadcrumb back-navigation and artifact link', async ({
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
		if (body.query.includes('ProjectRuns')) {
			const layer = body.variables?.['layer'] as string | undefined;
			const filtered = layer ? ALL_RUNS.filter((r) => r.layer === layer) : ALL_RUNS;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						runs: {
							edges: filtered.map(listRowFor),
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: filtered.length
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('ParentRunParent')) {
			const id = body.variables?.['id'] as string;
			const r = ALL_RUNS.find((n) => n.id === id);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r ? { parentRunId: r.parentRunId } : null } })
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
			const r = ALL_RUNS.find((n) => n.id === id) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r } })
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
		if (body.query.includes('ProducedArtifact')) {
			const id = body.variables?.['id'] as string;
			if (id === ARTIFACT_ID) {
				await route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({
						data: { artifact: { id: ARTIFACT_ID, title: 'Test Artifact' } }
					})
				});
				return;
			}
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { artifact: null } })
			});
			return;
		}
		await route.continue();
	});

	const listUrl = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs`;
	await page.goto(listUrl);

	// Step 1: list shows all 3 runs
	await expect(page.getByRole('heading', { name: 'Runs' })).toBeVisible();
	await expect(page.getByRole('row')).toHaveCount(4); // header + 3

	// Step 2: apply layer=work filter -> URL + filtered list
	await page.getByRole('button', { name: 'work', exact: true }).click();
	await expect(page).toHaveURL(/[?&]layer=work\b/);
	await expect(page.getByRole('row')).toHaveCount(2); // header + 1

	// Step 3: click work row's open-detail icon -> detail page
	await page.locator(`a[href$="/runs/${WORK_ID}"]`).first().click();
	await expect(page).toHaveURL(new RegExp(`/runs/${WORK_ID}(\\?.*)?$`));
	await expect(page.locator('h1', { hasText: WORK_ID })).toBeVisible();
	// Tabbed UI is present and starts on overview
	await expect(page.locator('[data-testid="rundetail"]')).toBeVisible();
	await expect(page.locator('[data-testid="rundetail-overview"]')).toBeVisible();
	// Work-layer Overview shows stop condition
	await expect(page.getByText('queue-empty')).toBeVisible();

	// Step 4: navigate directly to try detail page
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${TRY_ID}`);
	await expect(page.locator('h1', { hasText: TRY_ID })).toBeVisible();
	// Try-layer Overview fields
	await expect(page.locator('[data-testid="rundetail-overview"]')).toBeVisible();
	await expect(page.getByText('abc123def')).toBeVisible(); // baseRevision
	await expect(page.getByText('def456abc')).toBeVisible(); // resultRevision
	await expect(page.getByText('merged')).toBeVisible(); // mergeOutcome
	await expect(page.getByText('/tmp/ddx-exec-wt/.try-001')).toBeVisible(); // worktreePath
	// Bead link in Overview
	await expect(page.locator(`a[href$="/beads/${BEAD_ID}"]`).first()).toBeVisible();

	// Step 5: navigate to run detail page
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}`);
	await expect(page.locator('h1', { hasText: RUN_ID })).toBeVisible();
	// Run-layer Overview fields
	await expect(page.locator('[data-testid="rundetail-overview"]')).toBeVisible();
	await expect(page.getByText('claude', { exact: true }).first()).toBeVisible(); // harness
	await expect(page.getByText('12,000')).toBeVisible(); // tokens in
	await expect(page.getByText('3,400')).toBeVisible(); // tokens out
	await expect(page.getByText('$0.0876')).toBeVisible(); // cost

	// Step 6: artifact link present and live
	const artifactLink = page.getByTestId('produced-artifact').getByRole('link');
	await expect(artifactLink).toBeVisible();
	await expect(artifactLink).toHaveAttribute('href', new RegExp(`/artifacts/${ARTIFACT_ID}$`));

	// Step 7: breadcrumb back-navigation: run -> try -> work -> list
	// On run detail, breadcrumbs are: Runs / workId / tryId / runId
	const tryCrumb = page.locator('nav a').filter({ hasText: TRY_ID }).first();
	await tryCrumb.click();
	await expect(page).toHaveURL(new RegExp(`/runs/${TRY_ID}$`));

	// On try detail, breadcrumbs are: Runs / workId / tryId
	const workCrumb = page.locator('nav a').filter({ hasText: WORK_ID }).first();
	await workCrumb.click();
	await expect(page).toHaveURL(new RegExp(`/runs/${WORK_ID}$`));

	// Step 8: browser back to filtered list — filter state restored from history
	await page.goBack(); // back to try
	await expect(page).toHaveURL(new RegExp(`/runs/${TRY_ID}$`));
	await page.goBack(); // back to run
	await expect(page).toHaveURL(new RegExp(`/runs/${RUN_ID}$`));
	await page.goBack(); // back to try (forward visit)
	await page.goBack(); // back to work (initial click)
	await page.goBack(); // back to filtered list
	await expect(page).toHaveURL(/[?&]layer=work\b/);
	await expect(page.getByRole('row')).toHaveCount(2); // header + 1 work record
});

test('bead detail shows linked runs and click navigates to run detail', async ({ page }) => {
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
		if (body.query.includes('BeadsByProject')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						beadsByProject: {
							edges: [],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 0
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('Bead(')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						bead: {
							id: BEAD_ID,
							title: 'Runs cross-link test bead',
							status: 'open',
							priority: 2,
							issueType: 'task',
							owner: null,
							createdAt: '2026-04-30T08:00:00Z',
							createdBy: null,
							updatedAt: '2026-04-30T08:00:00Z',
							labels: [],
							parent: null,
							description: '',
							acceptance: '',
							notes: '',
							dependencies: []
						},
						projectBeads: { edges: [] },
						beadExecutions: { edges: [], totalCount: 0 },
						beadRuns: {
							edges: [
								{
									node: {
										id: TRY_ID,
										layer: 'try',
										status: 'success',
										harness: null,
										startedAt: tryNode.startedAt,
										durationMs: tryNode.durationMs
									}
								},
								{
									node: {
										id: RUN_ID,
										layer: 'run',
										status: 'success',
										harness: 'claude',
										startedAt: runNode.startedAt,
										durationMs: runNode.durationMs
									}
								}
							],
							totalCount: 2
						}
					}
				})
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
			const r = ALL_RUNS.find((n) => n.id === id) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r } })
			});
			return;
		}
		if (body.query.includes('ParentRunParent')) {
			const id = body.variables?.['id'] as string;
			const r = ALL_RUNS.find((n) => n.id === id);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r ? { parentRunId: r.parentRunId } : null } })
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

	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/beads/${BEAD_ID}`);

	const linkedRuns = page.getByTestId('bead-linked-runs');
	await expect(linkedRuns).toBeVisible();
	await expect(linkedRuns).toContainText(TRY_ID);
	await expect(linkedRuns).toContainText(RUN_ID);

	// Click try-layer linked run -> navigates to run detail
	await linkedRuns.locator(`a[href$="/runs/${TRY_ID}"]`).click();
	await expect(page).toHaveURL(new RegExp(`/runs/${TRY_ID}$`));
	await expect(page.locator('h1', { hasText: TRY_ID })).toBeVisible();
});

test('run detail page tabbed UI: 5 tabs, URL-driven tab state, navigation', async ({ page }) => {
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
		if (body.query.includes('RunHeader') || body.query.includes('RunDetailExpand')) {
			const id = body.variables?.['id'] as string;
			const r = ALL_RUNS.find((n) => n.id === id) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r } })
			});
			return;
		}
		if (body.query.includes('ParentRunParent')) {
			const id = body.variables?.['id'] as string;
			const r = ALL_RUNS.find((n) => n.id === id);
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: r ? { parentRunId: r.parentRunId } : null } })
			});
			return;
		}
		if (body.query.includes('RunExecutionExpand')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						execution: {
							id: 'exec-x',
							sessionId: 'sess-x',
							bundlePath: '/bundle',
							promptPath: '/bundle/prompt.md',
							manifestPath: null,
							resultPath: '/bundle/result.json',
							agentLogPath: null,
							prompt: 'sample prompt body',
							manifest: null,
							result: 'sample result body',
							rationale: null
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('RunSessionExpand')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentSession: {
							id: 'sess-x',
							harness: 'claude',
							model: 'claude-sonnet-4-6',
							cost: 0.0876,
							billingMode: 'usage',
							tokens: { prompt: 12000, completion: 3400, total: 15400, cached: 0 },
							status: 'completed',
							outcome: 'success',
							prompt: 'session prompt',
							response: 'session response',
							stderr: null
						}
					}
				})
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

	// Navigate directly to a run-layer detail page (run-layer has all 5 tabs)
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}`);
	await expect(page.locator('h1', { hasText: RUN_ID })).toBeVisible();

	const detail = page.locator('[data-testid="rundetail"]');
	await expect(detail).toBeVisible();

	// AC1: tabbed UI with all 5 tabs visible for run-layer
	await expect(detail.locator('button[data-tab="overview"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="prompt"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="response"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="tools"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="session"]')).toBeVisible();

	// Default tab is overview
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'overview');
	await expect(detail.locator('[data-testid="rundetail-overview"]')).toBeVisible();

	// AC2: clicking a tab updates URL
	await detail.locator('button[data-tab="prompt"]').click();
	await expect(page).toHaveURL(/[?&]tab=prompt\b/);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'prompt');
	await expect(detail.locator('[data-testid="rundetail-prompt"]')).toBeVisible();

	await detail.locator('button[data-tab="response"]').click();
	await expect(page).toHaveURL(/[?&]tab=response\b/);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'response');

	await detail.locator('button[data-tab="tools"]').click();
	await expect(page).toHaveURL(/[?&]tab=tools\b/);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'tools');

	await detail.locator('button[data-tab="session"]').click();
	await expect(page).toHaveURL(/[?&]tab=session\b/);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'session');

	// AC2: returning to overview removes tab from URL
	await detail.locator('button[data-tab="overview"]').click();
	await expect(page).not.toHaveURL(/[?&]tab=/);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'overview');

	// AC2: deep-link with ?tab=prompt opens that tab on load
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}?tab=prompt`);
	await expect(page.locator('[data-testid="rundetail"] [data-active-tab]')).toHaveAttribute(
		'data-active-tab',
		'prompt'
	);
	await expect(page.locator('[data-testid="rundetail-prompt"]')).toBeVisible();

	// Work-layer detail: only overview tab is shown
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${WORK_ID}`);
	await expect(page.locator('h1', { hasText: WORK_ID })).toBeVisible();
	const workDetail = page.locator('[data-testid="rundetail"]');
	await expect(workDetail.locator('button[data-tab="overview"]')).toBeVisible();
	await expect(workDetail.locator('button[data-tab="prompt"]')).toHaveCount(0);
	await expect(workDetail.locator('button[data-tab="response"]')).toHaveCount(0);
	await expect(workDetail.locator('button[data-tab="session"]')).toHaveCount(0);
	await expect(workDetail.locator('button[data-tab="tools"]')).toHaveCount(0);

	// Try-layer detail: overview, prompt, response, tools (no session)
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${TRY_ID}`);
	await expect(page.locator('h1', { hasText: TRY_ID })).toBeVisible();
	const tryDetail = page.locator('[data-testid="rundetail"]');
	await expect(tryDetail.locator('button[data-tab="overview"]')).toBeVisible();
	await expect(tryDetail.locator('button[data-tab="prompt"]')).toBeVisible();
	await expect(tryDetail.locator('button[data-tab="response"]')).toBeVisible();
	await expect(tryDetail.locator('button[data-tab="tools"]')).toBeVisible();
	await expect(tryDetail.locator('button[data-tab="session"]')).toHaveCount(0);
});
