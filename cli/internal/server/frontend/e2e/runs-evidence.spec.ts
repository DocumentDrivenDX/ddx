import { expect, test } from '@playwright/test';

let NODE_INFO: { id: string; name: string };
let PROJECT_ID: string;
let PROJECTS: Array<{ id: string; name: string; path: string }>;

const RUN_ID = 'run-try-evidence-001';
const BEAD_ID = 'ddx-evidence-test';

type BundleFileFixture = {
	path: string;
	size: number;
	mimeType: string;
};

type RunBundleFileContentFixture = {
	path: string;
	content: string | null;
	sizeBytes: number;
	truncated: boolean;
	mimeType: string;
};

const baseRun = {
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
	prompt: null,
	response: null,
	stderr: null,
	billingMode: null,
	outcome: null,
	detail: null,
	cachedTokens: null
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

function runDetailResponse() {
	return {
		...baseRun,
		projectID: PROJECT_ID
	};
}

async function installRunRoutes(
	page: import('@playwright/test').Page,
	options: {
		bundleFiles: BundleFileFixture[];
		bundleContent: Record<string, RunBundleFileContentFixture>;
		evidenceRequests: string[];
	}
) {
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
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: runDetailResponse() } })
			});
			return;
		}
		if (body.query.includes('RunEvidenceFiles')) {
			options.evidenceRequests.push(String(body.variables?.['id'] ?? ''));
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						run: {
							id: RUN_ID,
							bundleFiles: options.bundleFiles
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
						runToolCalls: {
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
			const path = String(body.variables?.['path'] ?? '');
			const payload = options.bundleContent[path];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { runBundleFile: payload ?? null } })
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
}

test('evidence tab: whitelisted small file inlines', async ({ page }) => {
	const evidenceRequests: string[] = [];
	await installRunRoutes(page, {
		bundleFiles: [
			{ path: 'manifest.json', size: 256, mimeType: 'application/json' },
			{ path: 'prompt.md', size: 128, mimeType: 'text/markdown' },
			{ path: 'screenshots/big.png', size: 200_000, mimeType: 'image/png' }
		],
		bundleContent: {
			'manifest.json': {
				path: 'manifest.json',
				content: '{"attempt":"20260430T100000"}',
				sizeBytes: 256,
				truncated: false,
				mimeType: 'application/json'
			},
			'prompt.md': {
				path: 'prompt.md',
				content: '# Sample prompt\nbody text',
				sizeBytes: 128,
				truncated: false,
				mimeType: 'text/markdown'
			},
			'screenshots/big.png': {
				path: 'screenshots/big.png',
				content: null,
				sizeBytes: 200_000,
				truncated: true,
				mimeType: 'image/png'
			}
		},
		evidenceRequests
	});

	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}`);
	const detail = page.locator('[data-testid="rundetail"]');
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'overview');
	await expect(evidenceRequests).toHaveLength(0);
	await expect(detail.locator('button[data-tab="evidence"]')).toBeVisible();

	await Promise.all([
		page.waitForRequest((request) => request.postData()?.includes('RunEvidenceFiles') ?? false),
		detail.locator('button[data-tab="evidence"]').click()
	]);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'evidence');

	const evidence = page.locator('[data-testid="rundetail-evidence"]');
	await expect(evidence.locator('[data-evidence-path="manifest.json"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-path="prompt.md"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-path="screenshots/big.png"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-view="prompt.md"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-view="manifest.json"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-view="screenshots/big.png"]')).toHaveCount(0);

	await evidence.locator('[data-evidence-view="prompt.md"]').click();
	await expect(evidence.locator('[data-testid="evidence-inline-content"]')).toContainText(
		'Sample prompt'
	);
	await expect(evidence.locator('[data-evidence-download="prompt.md"]')).toHaveAttribute(
		'href',
		`/api/runs/${encodeURIComponent(RUN_ID)}/bundle?path=${encodeURIComponent('prompt.md')}`
	);
});

test('evidence tab: large file download-only', async ({ page }) => {
	await installRunRoutes(page, {
		bundleFiles: [
			{ path: 'manifest.json', size: 256, mimeType: 'application/json' },
			{ path: 'screenshots/big.png', size: 200_000, mimeType: 'image/png' }
		],
		bundleContent: {
			'manifest.json': {
				path: 'manifest.json',
				content: '{"attempt":"20260430T100000"}',
				sizeBytes: 256,
				truncated: false,
				mimeType: 'application/json'
			},
			'screenshots/big.png': {
				path: 'screenshots/big.png',
				content: null,
				sizeBytes: 200_000,
				truncated: true,
				mimeType: 'image/png'
			}
		},
		evidenceRequests: []
	});

	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}?tab=evidence`);
	const evidence = page.locator('[data-testid="rundetail-evidence"]');
	await expect(evidence.locator('[data-evidence-path="screenshots/big.png"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-view="screenshots/big.png"]')).toHaveCount(0);
	await expect(evidence.locator('[data-evidence-download="screenshots/big.png"]')).toHaveAttribute(
		'href',
		`/api/runs/${encodeURIComponent(RUN_ID)}/bundle?path=${encodeURIComponent('screenshots/big.png')}`
	);
	await expect(evidence.locator('[data-testid="evidence-inline"]')).toHaveCount(0);
});

test('evidence tab: non-whitelisted extension download-only', async ({ page }) => {
	await installRunRoutes(page, {
		bundleFiles: [
			{ path: 'src/main.go', size: 432, mimeType: 'text/x-go' },
			{ path: 'module.wasm', size: 12_345, mimeType: 'application/wasm' }
		],
		bundleContent: {
			'src/main.go': {
				path: 'src/main.go',
				content: 'package main\n',
				sizeBytes: 432,
				truncated: false,
				mimeType: 'text/x-go'
			},
			'module.wasm': {
				path: 'module.wasm',
				content: null,
				sizeBytes: 12_345,
				truncated: true,
				mimeType: 'application/wasm'
			}
		},
		evidenceRequests: []
	});

	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}?tab=evidence`);
	const evidence = page.locator('[data-testid="rundetail-evidence"]');
	await expect(evidence.locator('[data-evidence-path="src/main.go"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-path="module.wasm"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-view="src/main.go"]')).toHaveCount(0);
	await expect(evidence.locator('[data-evidence-view="module.wasm"]')).toHaveCount(0);
	await expect(evidence.locator('[data-evidence-download="src/main.go"]')).toBeVisible();
	await expect(evidence.locator('[data-evidence-download="module.wasm"]')).toBeVisible();
});

test('p95 dataset: 200 tool calls and 30 evidence files render interactively', async ({ page }) => {
	const TOOL_CALL_TOTAL = 200;
	const TOOL_CALL_PAGE_SIZE = 50;
	const EVIDENCE_FILE_COUNT = 30;

	// 30 evidence files: 5 named whitelisted files + 25 additional .txt files
	const evidenceFiles: BundleFileFixture[] = [
		{ path: 'manifest.json', size: 256, mimeType: 'application/json' },
		{ path: 'prompt.md', size: 512, mimeType: 'text/markdown' },
		{ path: 'result.json', size: 1024, mimeType: 'application/json' },
		{ path: 'notes.txt', size: 200, mimeType: 'text/plain' },
		{ path: 'output.md', size: 400, mimeType: 'text/markdown' },
		...Array.from({ length: 25 }, (_, i) => ({
			path: `attachments/file_${String(i + 1).padStart(2, '0')}.txt`,
			size: 100 + i * 20,
			mimeType: 'text/plain'
		}))
	];

	// Paginated tool call response: 50 per page, cursor-based
	function makeToolCallPage(afterCursor: string | null) {
		const offset = afterCursor ? parseInt(afterCursor.replace('cur-', ''), 10) : 0;
		const start = offset + 1;
		const end = Math.min(offset + TOOL_CALL_PAGE_SIZE, TOOL_CALL_TOTAL);
		const edges = Array.from({ length: end - start + 1 }, (_, i) => {
			const seq = start + i;
			return {
				node: {
					id: `tc-${seq}`,
					seq,
					name: `tool_${seq}`,
					inputs: `{"n":${seq}}`,
					output: `result_${seq}`,
					error: null,
					durationMs: 10 + seq
				},
				cursor: `cur-${end}`
			};
		});
		const hasNextPage = end < TOOL_CALL_TOTAL;
		return {
			edges,
			pageInfo: { hasNextPage, endCursor: hasNextPage ? `cur-${end}` : null },
			totalCount: TOOL_CALL_TOTAL
		};
	}

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
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: { ...baseRun, projectID: PROJECT_ID } } })
			});
			return;
		}
		if (body.query.includes('RunEvidenceFiles')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { run: { id: RUN_ID, bundleFiles: evidenceFiles } } })
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
			const after = (body.variables?.['after'] as string | null | undefined) ?? null;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { runToolCalls: makeToolCallPage(after) } })
			});
			return;
		}
		if (body.query.includes('RunBundleFileFetch')) {
			const path = String(body.variables?.['path'] ?? '');
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						runBundleFile: {
							path,
							content: `content of ${path}`,
							sizeBytes: 128,
							truncated: false,
							mimeType: 'text/plain'
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

	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${RUN_ID}`);
	const detail = page.locator('[data-testid="rundetail"]');

	// All tabs visible immediately — no spinner blocking interaction
	await expect(detail.locator('button[data-tab="overview"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="prompt"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="response"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="tools"]')).toBeVisible();
	await expect(detail.locator('button[data-tab="evidence"]')).toBeVisible();

	// Tools tab: first page of 50 out of 200 renders and load-more is available
	await Promise.all([
		page.waitForRequest((req) => req.postData()?.includes('RunToolCallsExpand') ?? false),
		detail.locator('button[data-tab="tools"]').click()
	]);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'tools');
	const tools = page.locator('[data-testid="rundetail-tools"]');
	await expect(tools).toContainText('50 of 200 tool calls');
	await expect(tools.locator('[data-tool-seq="1"]')).toBeVisible();
	await expect(tools.locator('[data-tool-seq="50"]')).toBeVisible();
	await expect(tools.getByRole('button', { name: 'Load more' })).toBeVisible();

	// Evidence tab: all 30 files listed
	await Promise.all([
		page.waitForRequest((req) => req.postData()?.includes('RunEvidenceFiles') ?? false),
		detail.locator('button[data-tab="evidence"]').click()
	]);
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'evidence');
	const evidence = page.locator('[data-testid="rundetail-evidence"]');
	for (const f of evidenceFiles) {
		await expect(evidence.locator(`[data-evidence-path="${f.path}"]`)).toBeVisible();
	}
	await expect(evidence.locator('[data-evidence-path]')).toHaveCount(EVIDENCE_FILE_COUNT);
});

test('full stack: prompt/response/tool-trace/evidence', async ({ page }) => {
	const FS_RUN_ID = 'run-fullstack-001';

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
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						run: {
							...baseRun,
							id: FS_RUN_ID,
							projectID: PROJECT_ID,
							layer: 'try',
							prompt: 'full stack prompt content',
							response: 'full stack response content'
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
						runToolCalls: {
							edges: [
								{
									node: {
										id: 'rtc-fs-0',
										seq: 0,
										name: 'Read',
										inputs: JSON.stringify({ path: 'prompt.md' }),
										output: 'tool output content',
										error: null,
										durationMs: 10
									},
									cursor: 'rtc-fs-0'
								}
							],
							pageInfo: { hasNextPage: false, endCursor: null },
							totalCount: 1
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('RunEvidenceFiles')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					data: {
						run: {
							id: FS_RUN_ID,
							bundleFiles: [
								{ path: 'manifest.json', size: 256, mimeType: 'application/json' },
								{ path: 'prompt.md', size: 128, mimeType: 'text/markdown' },
								{ path: 'result.json', size: 140, mimeType: 'application/json' }
							]
						}
					}
				})
			});
			return;
		}
		if (body.query.includes('RunBundleFileFetch')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { runBundleFile: null } })
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

	await page.goto(`/nodes/${NODE_INFO.id}/projects/${PROJECT_ID}/runs/${FS_RUN_ID}`);
	const detail = page.locator('[data-testid="rundetail"]');
	await expect(detail).toBeVisible();

	// Prompt tab: prompt content visible
	await detail.locator('button[data-tab="prompt"]').click();
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'prompt');
	await expect(detail.locator('[data-testid="rundetail-prompt-body"]')).toContainText(
		'full stack prompt content'
	);

	// Response tab: response content visible
	await detail.locator('button[data-tab="response"]').click();
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'response');
	await expect(detail.locator('[data-testid="rundetail-response-body"]')).toContainText(
		'full stack response content'
	);

	// Tools tab: tool trace visible with at least one entry
	await detail.locator('button[data-tab="tools"]').click();
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'tools');
	await expect(detail.locator('[data-testid="rundetail-tools"]')).toBeVisible();
	await expect(detail.locator('[data-tool-seq="0"]')).toBeVisible();

	// Evidence tab: all three evidence files listed
	await detail.locator('button[data-tab="evidence"]').click();
	await expect(detail.locator('[data-active-tab]')).toHaveAttribute('data-active-tab', 'evidence');
	await expect(detail.locator('[data-testid="rundetail-evidence"]')).toBeVisible();
	await expect(detail.locator('[data-evidence-path="manifest.json"]')).toBeVisible();
	await expect(detail.locator('[data-evidence-path="prompt.md"]')).toBeVisible();
	await expect(detail.locator('[data-evidence-path="result.json"]')).toBeVisible();
});
