import { expect, test } from '@playwright/test';

// Fixture IDs resolved per-test from the live harness (see beforeEach).
let NODE_INFO: { id: string; name: string };
let PROJECT_ID: string;
let PROJECTS: Array<{ id: string; name: string; path: string }>;
let BASE_URL: string;

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
	PROJECTS = [{ id: PROJECT_ID, name: ids.projectName, path: ids.projectPath }];
	BASE_URL = `/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/executions`;
});

type ExecutionNode = {
	id: string;
	projectId: string;
	beadId: string | null;
	beadTitle: string | null;
	sessionId: string | null;
	workerId: string | null;
	harness: string | null;
	model: string | null;
	verdict: string | null;
	status: string | null;
	rationale: string | null;
	createdAt: string;
	startedAt: string | null;
	finishedAt: string | null;
	durationMs: number | null;
	costUsd: number | null;
	tokens: number | null;
	exitCode: number | null;
	baseRev: string | null;
	resultRev: string | null;
	bundlePath: string;
	promptPath: string | null;
	manifestPath: string | null;
	resultPath: string | null;
	agentLogPath: string | null;
	prompt?: string | null;
	manifest?: string | null;
	result?: string | null;
};

const passExec: ExecutionNode = {
	id: '20260422T100000-aaaa1111',
	projectId: '__fixture__',
	beadId: 'ddx-alpha',
	beadTitle: 'Alpha bead title',
	sessionId: 'eb-aaaa1111',
	workerId: null,
	harness: 'claude',
	model: 'claude-sonnet-4-6',
	verdict: 'PASS',
	status: 'success',
	rationale: 'all checks passed',
	createdAt: '2026-04-22T10:00:00Z',
	startedAt: '2026-04-22T10:00:00Z',
	finishedAt: '2026-04-22T10:01:00Z',
	durationMs: 60000,
	costUsd: 0.012,
	tokens: 1500,
	exitCode: 0,
	baseRev: 'abc123',
	resultRev: 'def456',
	bundlePath: '.ddx/executions/20260422T100000-aaaa1111',
	promptPath: '.ddx/executions/20260422T100000-aaaa1111/prompt.md',
	manifestPath: '.ddx/executions/20260422T100000-aaaa1111/manifest.json',
	resultPath: '.ddx/executions/20260422T100000-aaaa1111/result.json',
	agentLogPath: '.ddx/agent-logs/agent-eb-aaaa1111.jsonl',
	prompt: 'seeded prompt body for alpha',
	manifest: '{"attempt_id":"20260422T100000-aaaa1111","bead_id":"ddx-alpha"}',
	result: '{"verdict":"PASS","rationale":"all checks passed"}'
};

const blockExec: ExecutionNode = {
	id: '20260422T110000-bbbb2222',
	projectId: '__fixture__',
	beadId: 'ddx-alpha',
	beadTitle: 'Alpha bead title',
	sessionId: 'eb-bbbb2222',
	workerId: null,
	harness: 'codex',
	model: 'gpt-5.4',
	verdict: 'BLOCK',
	status: 'failure',
	rationale: 'gate failed',
	createdAt: '2026-04-22T11:00:00Z',
	startedAt: null,
	finishedAt: null,
	durationMs: 30000,
	costUsd: 0.04,
	tokens: 4000,
	exitCode: 1,
	baseRev: 'abc124',
	resultRev: null,
	bundlePath: '.ddx/executions/20260422T110000-bbbb2222',
	promptPath: '.ddx/executions/20260422T110000-bbbb2222/prompt.md',
	manifestPath: '.ddx/executions/20260422T110000-bbbb2222/manifest.json',
	resultPath: '.ddx/executions/20260422T110000-bbbb2222/result.json',
	agentLogPath: null,
	prompt: 'seeded prompt body for blocked attempt',
	manifest: '{"attempt_id":"20260422T110000-bbbb2222","bead_id":"ddx-alpha"}',
	result: '{"verdict":"BLOCK","rationale":"gate failed"}'
};

const passExec2: ExecutionNode = {
	...passExec,
	id: '20260422T120000-cccc3333',
	beadId: 'ddx-beta',
	beadTitle: 'Beta bead title',
	sessionId: 'eb-cccc3333',
	createdAt: '2026-04-22T12:00:00Z',
	bundlePath: '.ddx/executions/20260422T120000-cccc3333'
};

function executionsListPayload(rows: ExecutionNode[], filtered?: (e: ExecutionNode) => boolean) {
	const rowsOut = filtered ? rows.filter(filtered) : rows;
	return {
		executions: {
			edges: rowsOut.map((node) => ({
				node: {
					id: node.id,
					projectId: PROJECT_ID,
					beadId: node.beadId,
					beadTitle: node.beadTitle,
					sessionId: node.sessionId,
					harness: node.harness,
					model: node.model,
					verdict: node.verdict,
					status: node.status,
					createdAt: node.createdAt,
					startedAt: node.startedAt,
					finishedAt: node.finishedAt,
					durationMs: node.durationMs,
					costUsd: node.costUsd,
					tokens: node.tokens,
					exitCode: node.exitCode,
					bundlePath: node.bundlePath
				},
				cursor: node.id
			})),
			pageInfo: { hasNextPage: false, hasPreviousPage: false, startCursor: null, endCursor: null },
			totalCount: rowsOut.length
		}
	};
}

test('executions list and detail flow', async ({ page }) => {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as { query: string; variables?: Record<string, unknown> };
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { nodeInfo: NODE_INFO } })
			});
			return;
		}
		if (body.query.includes('Projects')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((node) => ({ node })) } } })
			});
			return;
		}
		if (body.query.includes('ExecutionsPage')) {
			const all = [passExec2, blockExec, passExec];
			const verdict = body.variables?.['verdict'] as string | undefined;
			const filtered = verdict ? all.filter((e) => e.verdict === verdict) : all;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: executionsListPayload(filtered) })
			});
			return;
		}
		if (body.query.includes('ExecutionDetail')) {
			const id = body.variables?.['id'] as string;
			const exec = [passExec2, blockExec, passExec].find((e) => e.id === id) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { execution: exec } })
			});
			return;
		}
		if (body.query.includes('ExecutionToolCalls')) {
			const calls = Array.from({ length: 3 }).map((_, i) => ({
				node: {
					id: `tc-${i}`,
					name: 'Bash',
					seq: i,
					ts: '2026-04-22T11:00:00Z',
					inputs: JSON.stringify({ command: `echo step ${i}` }),
					output: `step-${i}-output`,
					truncated: false
				},
				cursor: `tc-${i}`
			}));
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						executionToolCalls: {
							edges: calls,
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: calls.length
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('ExecutionSession')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						agentSession: {
							id: 'eb-bbbb2222',
							harness: 'codex',
							model: 'gpt-5.4',
							cost: 0.04,
							billingMode: 'paid',
							tokens: { prompt: 1000, completion: 3000, total: 4000, cached: 0 },
							status: 'failed',
							outcome: 'failure'
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
							id: 'ddx-alpha',
							title: 'Alpha bead title',
							status: 'open',
							priority: 2,
							issueType: 'task',
							owner: null,
							createdAt: '2026-04-22T08:00:00Z',
							createdBy: null,
							updatedAt: '2026-04-22T08:00:00Z',
							labels: [],
							parent: null,
							description: '',
							acceptance: '',
							notes: '',
							dependencies: []
						},
						projectBeads: { edges: [] },
						beadExecutions: {
							edges: [
								{ node: { id: passExec.id, verdict: 'PASS', harness: 'claude', createdAt: passExec.createdAt, durationMs: 60000, costUsd: 0.012 } },
								{ node: { id: blockExec.id, verdict: 'BLOCK', harness: 'codex', createdAt: blockExec.createdAt, durationMs: 30000, costUsd: 0.04 } }
							],
							totalCount: 2
						}
					}
				})
			});
			return;
		}
		await route.continue();
	});

	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Executions' })).toBeVisible();
	// 3 rows expected (one per execution). The list cells show the formatted
	// timestamp; the row's link href is the only reliable carrier of the
	// execution ID, so we locate by href.
	await expect(page.getByRole('row')).toHaveCount(4); // 1 header + 3 data rows
	const passLink = page.locator(`a[href$="/executions/${passExec.id}"]`);
	const blockLink = page.locator(`a[href$="/executions/${blockExec.id}"]`);
	await expect(passLink.first()).toBeVisible();
	await expect(blockLink.first()).toBeVisible();

	// Filter by verdict=BLOCK.
	await page.getByLabel('Verdict').selectOption('BLOCK');
	await page.getByRole('button', { name: 'Apply' }).click();
	await expect(page.getByRole('row')).toHaveCount(2); // header + 1
	await expect(blockLink.first()).toBeVisible();
	await expect(passLink).toHaveCount(0);

	// Drill into the BLOCK execution detail.
	await blockLink.first().click();
	await expect(page.getByRole('heading', { name: blockExec.id })).toBeVisible();

	// Default tab is Manifest.
	await expect(page.getByTestId('manifest-body')).toContainText('bbbb2222');

	// Switch to Prompt and verify.
	await page.getByRole('button', { name: 'Prompt' }).click();
	await expect(page.getByTestId('prompt-body')).toContainText('seeded prompt body for blocked attempt');

	// Switch to Result.
	await page.getByRole('button', { name: 'Result' }).click();
	await expect(page.getByTestId('result-body')).toContainText('gate failed');

	// Switch to Session — fetches in the background.
	await page.getByRole('button', { name: 'Session' }).click();
	await expect(page.getByText('eb-bbbb2222', { exact: false })).toBeVisible();

	// Switch to Tool calls — first page loads.
	await page.getByRole('button', { name: 'Tool calls' }).click();
	await expect(page.getByText('3 of 3 tool calls')).toBeVisible();
	// Expand first tool call.
	await page.locator('[data-tool-seq="0"]').click();
	await expect(page.getByText('echo step 0')).toBeVisible();
	await expect(page.getByText('step-0-output')).toBeVisible();

	// Navigate to the bead from the detail page and confirm executions section
	// lists this attempt.
	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/beads/ddx-alpha`);
	await expect(page.getByTestId('bead-executions')).toBeVisible();
	await expect(page.getByTestId('bead-executions')).toContainText(passExec.id);
	await expect(page.getByTestId('bead-executions')).toContainText(blockExec.id);
});
