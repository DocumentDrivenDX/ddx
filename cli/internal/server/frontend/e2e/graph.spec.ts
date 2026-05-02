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
	issueId?: string;
	kind: string;
	path: string | null;
	id: string | null;
	message: string;
	relatedPath: string | null;
}

function makeGraphResponse(
	docs: readonly { id: string; path: string; title: string; dependsOn: string[]; dependents: string[]; staleness?: string }[] = GRAPH_DOCS,
	warnings: string[] = [],
	issues: GraphIssueFixture[] = []
) {
	const pathToId: Record<string, string> = {};
	for (const doc of docs) {
		pathToId[doc.path] = doc.id;
	}
	return {
		docGraph: {
			rootDir: '/repos/alpha',
			documents: docs,
			pathToId: JSON.stringify(pathToId),
			warnings,
			issues
		}
	};
}

/**
 * Set up GraphQL route mocking for the graph page.
 */
type FixtureDoc = (typeof GRAPH_DOCS)[number] & { staleness?: string };

async function mockGraphQL(
	page: import('@playwright/test').Page,
	docs: readonly FixtureDoc[] = GRAPH_DOCS,
	warnings: string[] = [],
	issues: GraphIssueFixture[] = [],
	staleDocIds: string[] = []
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
		} else if (body.query.includes('DocStale')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { docStale: staleDocIds.map((id) => ({ id })) } })
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
		try {
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
		} catch {
			await route.abort('failed').catch(() => {});
		}
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
		await page.unroute('/graphql');
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
		await page.unroute('/graphql');
		await stopRealDdxServer(server);
	}
});

// TC-039: Clicking a graph node navigates to the document page
test('TC-039: clicking a graph node navigates to document detail page', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto(BASE_URL);

	await expect(page.getByTestId('doc-graph-svg')).toBeVisible();

	// Wait for force simulation and auto-fit to settle
	await page.waitForTimeout(800);

	// Click the first circle node in the SVG (DOM order matches data binding =
	// GRAPH_DOCS[0] = doc-001 with path "docs/vision.md")
	await page.locator('[data-testid="doc-graph-svg"] circle').first().click();

	// Navigation should go to the document page at the node's specific path
	// (SPA navigation, no full reload)
	await expect(page).toHaveURL(`${BASE_URL.replace('/graph', '')}/documents/docs/vision.md`);
});

// TC-040: Back navigation restores graph viewport
test('TC-040: Back navigation restores graph viewport from URL params', async ({ page }) => {
	await mockGraphQL(page);

	// Navigate to graph with preset viewport params
	await page.goto(`${BASE_URL}?zoom=2.000&pan=100.0,50.0`);

	await expect(page.getByTestId('doc-graph-svg')).toBeVisible();
	await page.waitForTimeout(500);

	// Click a node to navigate to the document page
	await page.locator('[data-testid="doc-graph-svg"] circle').first().click();
	await expect(page).toHaveURL(/\/documents\//);

	// Navigate back
	await page.goBack();

	// Verify viewport params are preserved in the URL
	await expect(page).toHaveURL(/zoom=2\.000/);
	await expect(page).toHaveURL(/pan=100\.0%2C50\.0|pan=100\.0,50\.0/);
});

// TC-041: Staleness filter chips update visible node count
test('TC-041: staleness filter chips update visible node count', async ({ page }) => {
	// Mark doc-001 as stale; doc-002 and doc-003 are fresh
	await mockGraphQL(page, GRAPH_DOCS, [], [], ['doc-001']);
	await page.goto(BASE_URL);

	await expect(page.getByRole('heading', { name: 'Document Graph' })).toBeVisible();

	// All 3 nodes visible initially
	await expect(page.getByText(/3 nodes/)).toBeVisible();

	// Click "Stale" filter chip — only stale nodes (1) should be visible
	await page.getByTestId('filter-staleness-stale').click();
	await expect(page.getByText(/1 nodes/)).toBeVisible();

	// Click "Fresh" filter chip as well — adds fresh nodes back (1 stale + 2 fresh = 3 would show
	// if both active, but only stale is active here... click to toggle fresh instead)
	// Remove stale filter
	await page.getByTestId('filter-staleness-stale').click();

	// Back to all 3 nodes
	await expect(page.getByText(/3 nodes/)).toBeVisible();
});

// TC-042: Edges and arrowheads remain visible in dark mode.
//
// Regression guard for the previous styling, which painted edges with
// border-line tokens at stroke-opacity 0.6 — alpha-blended that landed at
// roughly 1.1:1 against the canvas, well below the WCAG 3:1 non-text floor.
//
// We compute contrast against the actual canvas pixel the edge paints onto,
// folding any residual alpha/parent opacity into a flattened sRGB color before
// running the luminance ratio. Asserting on the resolved color (not the token
// string the source happens to reference) means the test catches both
// regressions: a token rename AND a silent alpha multiplier sneaking back in.
test('TC-042: edges and arrowheads meet WCAG 3:1 against canvas in dark mode', async ({
	page
}) => {
	await page.addInitScript(() => {
		window.localStorage.setItem('mode-watcher-mode', 'dark');
	});

	// Fixture renders fresh / stale / missing in one frame so the contrast
	// check exercises the same edge color across every node-state pairing the
	// graph can produce. doc-001=fresh, doc-002=stale, doc-003=missing.
	const STALENESS_FIXTURE_DOCS = [
		{
			id: 'doc-001',
			path: 'docs/vision.md',
			title: 'Vision',
			dependsOn: [],
			dependents: ['doc-002', 'doc-003'],
			staleness: 'fresh'
		},
		{
			id: 'doc-002',
			path: 'docs/prd.md',
			title: 'PRD',
			dependsOn: ['doc-001'],
			dependents: ['doc-003'],
			staleness: 'stale'
		},
		{
			id: 'doc-003',
			path: 'docs/design.md',
			title: 'Design',
			dependsOn: ['doc-001', 'doc-002'],
			dependents: [],
			staleness: 'missing'
		}
	];

	await mockGraphQL(page, STALENESS_FIXTURE_DOCS);
	await page.goto(BASE_URL);
	await expect(page.getByTestId('doc-graph-svg')).toBeVisible();
	// Let the simulation settle so edges have non-zero coordinates.
	await page.waitForTimeout(800);

	// All three staleness classes are in the DOM — confirms the fixture really
	// does render every state the test claims to cover.
	await expect(page.locator('[data-testid="doc-graph-svg"] circle.node-fresh')).toHaveCount(1);
	await expect(page.locator('[data-testid="doc-graph-svg"] circle.node-stale')).toHaveCount(1);
	await expect(page.locator('[data-testid="doc-graph-svg"] circle.node-missing')).toHaveCount(1);

	// Pull computed colors (already resolved through the CSS var cascade), the
	// canvas background, and any opacity applied to edges or their ancestors.
	// page.evaluate runs in the browser, so getComputedStyle gives us the real
	// painted RGB. We return raw strings and parse on the test side.
	const sample = await page.evaluate(() => {
		function readEffectiveOpacity(el: Element | null): number {
			let o = 1;
			let cur: Element | null = el;
			while (cur) {
				const cs = getComputedStyle(cur);
				const v = parseFloat(cs.opacity);
				if (!Number.isNaN(v)) o *= v;
				cur = cur.parentElement;
			}
			return o;
		}
		const svg = document.querySelector('[data-testid="doc-graph-svg"]') as SVGSVGElement | null;
		const line = svg?.querySelector('line.graph-edge') as SVGLineElement | null;
		const arrow = svg?.querySelector('marker#ddx-arrow path.graph-edge-arrow') as
			| SVGPathElement
			| null;
		const canvas = svg?.parentElement;
		if (!svg || !line || !arrow || !canvas) {
			return null;
		}
		const lineCs = getComputedStyle(line);
		const arrowCs = getComputedStyle(arrow);
		const canvasCs = getComputedStyle(canvas);
		return {
			edgeStroke: lineCs.stroke,
			arrowFill: arrowCs.fill,
			strokeOpacityAttr: line.getAttribute('stroke-opacity'),
			edgeOpacity: readEffectiveOpacity(line),
			arrowOpacity: readEffectiveOpacity(arrow),
			canvasBg: canvasCs.backgroundColor
		};
	});

	expect(sample, 'graph DOM should expose a graph-edge line and graph-edge-arrow path').not.toBeNull();
	const s = sample!;

	// AC2 belt-and-braces: no inline stroke-opacity attribute survives.
	expect(s.strokeOpacityAttr).toBeNull();

	function parseRgb(c: string): [number, number, number, number] {
		const m = /rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)\s*(?:,\s*([\d.]+))?\s*\)/.exec(c);
		if (!m) throw new Error(`unrecognised color: ${c}`);
		const a = m[4] !== undefined ? parseFloat(m[4]) : 1;
		return [parseFloat(m[1]), parseFloat(m[2]), parseFloat(m[3]), a];
	}

	// Alpha-composite a stroke color (with combined alpha) onto the canvas so
	// the contrast check reflects what the user actually sees.
	function flatten(
		fg: [number, number, number, number],
		bg: [number, number, number, number],
		extraAlpha: number
	): [number, number, number] {
		const a = fg[3] * extraAlpha;
		return [
			fg[0] * a + bg[0] * (1 - a),
			fg[1] * a + bg[1] * (1 - a),
			fg[2] * a + bg[2] * (1 - a)
		];
	}

	function srgb(c: number): number {
		const v = c / 255;
		return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
	}
	function lum(rgb: [number, number, number]): number {
		return 0.2126 * srgb(rgb[0]) + 0.7152 * srgb(rgb[1]) + 0.0722 * srgb(rgb[2]);
	}
	function ratio(a: [number, number, number], b: [number, number, number]): number {
		const la = lum(a);
		const lb = lum(b);
		const [hi, lo] = la > lb ? [la, lb] : [lb, la];
		return (hi + 0.05) / (lo + 0.05);
	}

	const bg = parseRgb(s.canvasBg);
	const bgRgb: [number, number, number] = [bg[0], bg[1], bg[2]];

	const edgeFlat = flatten(parseRgb(s.edgeStroke), bg, s.edgeOpacity);
	const arrowFlat = flatten(parseRgb(s.arrowFill), bg, s.arrowOpacity);

	expect(ratio(edgeFlat, bgRgb)).toBeGreaterThanOrEqual(3);
	expect(ratio(arrowFlat, bgRgb)).toBeGreaterThanOrEqual(3);
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
