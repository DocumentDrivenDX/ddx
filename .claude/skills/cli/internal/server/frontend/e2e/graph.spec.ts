import { expect, request as playwrightRequest, test } from '@playwright/test';
import type { APIRequestContext, Page } from '@playwright/test';
import { spawn, spawnSync, type ChildProcessWithoutNullStreams } from 'node:child_process';
import * as fs from 'node:fs';
import * as net from 'node:net';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

// Shared fixtures
const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/graph`;

const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];

const FRONTEND_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CLI_DIR = path.resolve(FRONTEND_DIR, '../../..');

let ddxBinary: string | null = null;

const GRAPH_DOCS = [
	{
		id: 'doc-001',
		path: 'docs/vision.md',
		title: 'Vision',
		dependsOn: [],
		dependents: ['doc-002', 'doc-003']
	},
	{
		id: 'doc-002',
		path: 'docs/prd.md',
		title: 'PRD',
		dependsOn: ['doc-001'],
		dependents: ['doc-003']
	},
	{
		id: 'doc-003',
		path: 'docs/design.md',
		title: 'Design',
		dependsOn: ['doc-001', 'doc-002'],
		dependents: []
	}
];

interface GraphIssueFixture {
	kind: string;
	path: string | null;
	id: string | null;
	message: string;
	relatedPath: string | null;
}

function makeGraphResponse(
	docs = GRAPH_DOCS,
	warnings: string[] = [],
	issues: GraphIssueFixture[] = []
) {
	return {
		docGraph: {
			rootDir: '/repos/alpha',
			documents: docs,
			warnings,
			issues
		}
	};
}

/**
 * Set up GraphQL route mocking for the graph page.
 */
async function mockGraphQL(
	page: import('@playwright/test').Page,
	docs = GRAPH_DOCS,
	warnings: string[] = [],
	issues: GraphIssueFixture[] = []
) {
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
		} else if (body.query.includes('DocGraph')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeGraphResponse(docs, warnings, issues) })
			});
		} else {
			await route.continue();
		}
	});
}

function ensureDdxBinary(): string {
	if (ddxBinary) return ddxBinary;

	const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ddx-graph-e2e-bin-'));
	ddxBinary = path.join(binDir, process.platform === 'win32' ? 'ddx-e2e.exe' : 'ddx-e2e');
	const result = spawnSync('go', ['build', '-o', ddxBinary, '.'], {
		cwd: CLI_DIR,
		env: process.env,
		encoding: 'utf8'
	});
	if (result.status !== 0) {
		throw new Error(`failed to build ddx test binary\n${result.stdout}\n${result.stderr}`);
	}
	return ddxBinary;
}

async function freePort(): Promise<number> {
	return new Promise((resolve, reject) => {
		const server = net.createServer();
		server.once('error', reject);
		server.listen(0, '127.0.0.1', () => {
			const address = server.address();
			if (!address || typeof address === 'string') {
				server.close(() => reject(new Error('could not allocate port')));
				return;
			}
			const port = address.port;
			server.close(() => resolve(port));
		});
	});
}

function writeFixtureFile(root: string, rel: string, content: string) {
	const target = path.join(root, ...rel.split('/'));
	fs.mkdirSync(path.dirname(target), { recursive: true });
	fs.writeFileSync(target, content);
}

function makeIssueFixture(): string {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ddx-graph-issues-'));
	writeFixtureFile(root, 'docs/alpha.md', '---\nddx:\n  id: shared.id\n---\n# Alpha\n');
	writeFixtureFile(root, 'docs/beta.md', '---\nddx:\n  id: shared.id\n---\n# Beta\n');
	writeFixtureFile(
		root,
		'docs/gamma.md',
		'---\nddx:\n  id: doc.gamma\n  depends_on:\n    - ghost.doc\n---\n# Gamma\n'
	);
	return root;
}

function makeCleanFixture(): string {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ddx-graph-clean-'));
	writeFixtureFile(root, 'docs/alpha.md', '---\nddx:\n  id: doc.alpha\n---\n# Alpha\n');
	writeFixtureFile(
		root,
		'docs/beta.md',
		'---\nddx:\n  id: doc.beta\n  depends_on:\n    - doc.alpha\n---\n# Beta\n'
	);
	return root;
}

interface RealServer {
	api: APIRequestContext;
	nodeId: string;
	projectId: string;
	process: ChildProcessWithoutNullStreams;
	root: string;
}

async function startRealDdxServer(fixtureRoot: string): Promise<RealServer> {
	const port = await freePort();
	const bin = ensureDdxBinary();
	const child = spawn(bin, ['server', '--port', String(port), '--tsnet=false'], {
		cwd: fixtureRoot,
		env: {
			...process.env,
			DDX_NODE_NAME: 'graph-e2e-node',
			XDG_DATA_HOME: path.join(fixtureRoot, '.xdg-data')
		}
	});
	child.stdout.resume();
	child.stderr.resume();
	const baseURL = `https://127.0.0.1:${port}`;
	const api = await playwrightRequest.newContext({ baseURL, ignoreHTTPSErrors: true });

	let lastError: unknown;
	for (let i = 0; i < 80; i++) {
		if (child.exitCode !== null) {
			throw new Error(`ddx server exited early with code ${child.exitCode}`);
		}
		try {
			const resp = await api.get('/api/health', { timeout: 500 });
			if (resp.ok()) {
				const infoResp = await api.post('/graphql', {
					data: {
						query: `query E2EProjectInfo {
							nodeInfo { id name }
							projects { edges { node { id name path } } }
						}`
					}
				});
				const payload = (await infoResp.json()) as {
					data: {
						nodeInfo: { id: string };
						projects: { edges: Array<{ node: { id: string } }> };
					};
				};
				const projectId = payload.data.projects.edges[0]?.node.id;
				if (!projectId) throw new Error('ddx server returned no registered project');
				return {
					api,
					nodeId: payload.data.nodeInfo.id,
					projectId,
					process: child,
					root: fixtureRoot
				};
			}
		} catch (err) {
			lastError = err;
		}
		await new Promise((resolve) => setTimeout(resolve, 125));
	}

	child.kill();
	await api.dispose();
	throw new Error(`ddx server did not become healthy: ${String(lastError)}`);
}

async function stopRealDdxServer(server: RealServer) {
	await server.api.dispose();
	if (server.process.exitCode === null) {
		server.process.kill();
		await Promise.race([
			new Promise((resolve) => server.process.once('exit', resolve)),
			new Promise((resolve) => {
				setTimeout(() => {
					if (server.process.exitCode === null) server.process.kill('SIGKILL');
					resolve(undefined);
				}, 2000);
			})
		]);
	}
	fs.rmSync(server.root, { recursive: true, force: true });
}

async function proxyGraphQLToRealServer(page: Page, api: APIRequestContext) {
	await page.route('/graphql', async (route) => {
		const response = await api.post('/graphql', {
			data: route.request().postDataJSON()
		});
		await route.fulfill({
			status: response.status(),
			headers: {
				'content-type': response.headers()['content-type'] ?? 'application/json'
			},
			body: await response.text()
		});
	});
}

// TC-030: Graph page loads with heading
test('TC-030: graph page loads with Document Graph heading', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();
});

// TC-031: Node and edge counts are displayed in the header
test('TC-031: graph page shows node and edge counts', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// 3 nodes, 3 edges (doc-002 depends on doc-001 = 1 edge, doc-003 depends on doc-001 + doc-002 = 2 edges)
	await expect(page.getByText(/3 nodes/)).toBeVisible();
	await expect(page.getByText(/3 edges/)).toBeVisible();
});

// TC-032: D3Graph canvas element is rendered when documents exist
test('TC-032: D3Graph SVG element is rendered for non-empty graph', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	// The D3Graph component renders an SVG distinct from navigation icons.
	await expect(page.getByTestId('doc-graph-svg')).toBeVisible();
});

// TC-033: Empty state is shown when no documents are in the graph
test('TC-033: graph page shows empty state when no documents', async ({ page }) => {
	await mockGraphQL(page, [], []);
	await page.goto(BASE_URL);

	await expect(page.getByText('No documents in graph.')).toBeVisible();

	// Node/edge counts should be 0 · 0
	await expect(page.getByText(/0 nodes/)).toBeVisible();
	await expect(page.getByText(/0 edges/)).toBeVisible();
});

// TC-034: Structured issues are surfaced in the integrity panel.
test('TC-034: structured issue messages appear in the integrity panel', async ({ page }) => {
	const issues: GraphIssueFixture[] = [
		{
			kind: 'cycle',
			path: null,
			id: null,
			message: 'cycle detected: doc-001 -> doc-002 -> doc-001',
			relatedPath: null
		},
		{
			kind: 'missing_dep',
			path: 'docs/orphan.md',
			id: 'ghost',
			message: 'document "docs/orphan.md" declares dependency "ghost" which is not in the graph',
			relatedPath: null
		}
	];
	await mockGraphQL(page, GRAPH_DOCS, [], issues);
	await page.goto(BASE_URL);

	// Expand groups to reveal messages.
	await page.getByTestId('integrity-group-cycle').click();
	await page.getByTestId('integrity-group-missing_dep').click();

	await expect(page.getByText('cycle detected: doc-001 -> doc-002 -> doc-001')).toBeVisible();
	await expect(
		page.getByText(
			'document "docs/orphan.md" declares dependency "ghost" which is not in the graph'
		)
	).toBeVisible();
});

// TC-035: No amber surface when graph has no issues
test('TC-035: integrity surface is absent when no issues are returned', async ({ page }) => {
	await mockGraphQL(page, GRAPH_DOCS, [], []);
	await page.goto(BASE_URL);

	// The integrity panel container should not be present
	await expect(page.getByTestId('integrity-panel')).toHaveCount(0);
});

// TC-037: Fixture-backed graph integrity uses real docgraph detection and GraphQL plumbing.
test('TC-037: integrity panel groups real fixture issues by kind with counts and paths', async ({
	page
}) => {
	const server = await startRealDdxServer(makeIssueFixture());
	try {
		await proxyGraphQLToRealServer(page, server.api);
		await page.goto(`/nodes/${server.nodeId}/projects/${server.projectId}/graph`);

		const panel = page.getByTestId('integrity-panel');
		await expect(panel).toBeVisible();
		await expect(panel).toContainText('Duplicate ID');
		await expect(panel).toContainText('(1)');
		await expect(panel).toContainText('Missing dep target');

		const badge = page.getByTestId('integrity-badge');
		await expect(badge).toBeVisible();
		await expect(badge).toContainText('2');

		// Expand Duplicate ID group and assert both fixture paths are visible.
		await page.getByTestId('integrity-group-duplicate_id').click();
		await expect(panel).toContainText('docs/alpha.md');
		await expect(panel).toContainText('docs/beta.md');

		// Expand Missing dep target group and assert the frontmatter removal snippet is visible.
		await page.getByTestId('integrity-group-missing_dep').click();
		await expect(panel.getByTestId('integrity-missing-dep-snippet')).toContainText('- ghost.doc');

		// Clicking the path link navigates to the real document viewer for that file.
		const pathLink = panel.getByTestId('integrity-path-link').first();
		const href = await pathLink.getAttribute('href');
		expect(href).toBe(
			`/nodes/${server.nodeId}/projects/${server.projectId}/documents/docs/beta.md`
		);

		await pathLink.click();
		await expect(page).toHaveURL(href!);
	} finally {
		await stopRealDdxServer(server);
	}
});

// TC-038: Clean graph hides both the badge and the integrity panel.
test('TC-038: clean graph hides the integrity badge and panel', async ({ page }) => {
	const server = await startRealDdxServer(makeCleanFixture());
	try {
		await proxyGraphQLToRealServer(page, server.api);
		await page.goto(`/nodes/${server.nodeId}/projects/${server.projectId}/graph`);

		await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();
		await expect(page.getByTestId('integrity-panel')).toHaveCount(0);
		await expect(page.getByTestId('integrity-badge')).toHaveCount(0);
	} finally {
		await stopRealDdxServer(server);
	}
});

// TC-036: Graph page re-fetches DocGraph query on navigation (interaction with query)
test('TC-036: graph page issues DocGraph query to load graph data', async ({ page }) => {
	let graphQueryCount = 0;

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
		} else if (body.query.includes('DocGraph')) {
			graphQueryCount++;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: makeGraphResponse() })
			});
		} else {
			await route.continue();
		}
	});

	await page.goto(BASE_URL);

	// Wait for the page to fully render
	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();

	// DocGraph query must have been called at least once to populate the page
	expect(graphQueryCount).toBeGreaterThanOrEqual(1);
});
