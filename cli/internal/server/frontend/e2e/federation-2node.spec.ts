// B14.8b: 2-node federation Playwright e2e suite + ts-net guard.
//
// Each test spins up its own pair of ddx-server processes (1 hub + 1 spoke)
// on loopback, both with --tsnet=false so no Tailscale dependency leaks
// in. Both nodes register as spokes against the hub (the hub uses the
// hub_spoke role) so the hub's /federation page lists both, and any
// ?scope=federation fan-out returns rows from both nodes.
//
// The "ts-net guard" tests use a separate hub bound to 0.0.0.0 with a
// non-loopback host IP discovered at runtime; the registration request is
// issued against that non-loopback IP so the hub's requireFederationTrusted
// gate sees a non-loopback RemoteAddr. If no non-loopback IPv4 is available
// (CI image without an external interface), the ts-net guard tests
// test.skip with a clear message — the loopback flows still cover the
// other AC items.

import { expect, request as playwrightRequest, test } from '@playwright/test';
import type { APIRequestContext } from '@playwright/test';
import { spawn, spawnSync, type ChildProcessWithoutNullStreams } from 'node:child_process';
import * as fs from 'node:fs';
import * as net from 'node:net';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const FRONTEND_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CLI_DIR = path.resolve(FRONTEND_DIR, '../../..');
const FIXTURE_DIR = path.resolve(FRONTEND_DIR, 'e2e/fixtures');

let ddxBinary: string | null = null;

function ensureDdxBinary(): string {
	if (ddxBinary) return ddxBinary;
	const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ddx-fed-e2e-bin-'));
	ddxBinary = path.join(binDir, process.platform === 'win32' ? 'ddx-fed-e2e.exe' : 'ddx-fed-e2e');
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
		const srv = net.createServer();
		srv.once('error', reject);
		srv.listen(0, '127.0.0.1', () => {
			const a = srv.address();
			if (!a || typeof a === 'string') {
				srv.close(() => reject(new Error('alloc port')));
				return;
			}
			const port = a.port;
			srv.close(() => resolve(port));
		});
	});
}

function copyFixture(): string {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ddx-fed-e2e-ws-'));
	const ent = (rel: string) => path.join(FIXTURE_DIR, rel);
	const dst = (rel: string) => path.join(root, rel);
	function walk(srcRel: string) {
		const stat = fs.statSync(ent(srcRel));
		if (stat.isDirectory()) {
			fs.mkdirSync(dst(srcRel), { recursive: true });
			for (const child of fs.readdirSync(ent(srcRel))) {
				walk(path.join(srcRel, child));
			}
		} else {
			fs.mkdirSync(path.dirname(dst(srcRel)), { recursive: true });
			fs.copyFileSync(ent(srcRel), dst(srcRel));
		}
	}
	for (const top of fs.readdirSync(FIXTURE_DIR)) {
		walk(top);
	}
	return root;
}

interface SpawnedServer {
	port: number;
	addr: string;
	baseURL: string;
	root: string;
	child: ChildProcessWithoutNullStreams;
	stdoutBuf: string;
	stderrBuf: string;
	api: APIRequestContext;
}

async function waitHealthy(api: APIRequestContext, child: ChildProcessWithoutNullStreams) {
	let last: unknown;
	for (let i = 0; i < 80; i++) {
		if (child.exitCode !== null) {
			throw new Error(`server exited early (${child.exitCode})`);
		}
		try {
			const r = await api.get('/api/health', { timeout: 500 });
			if (r.ok()) return;
		} catch (e) {
			last = e;
		}
		await new Promise((r) => setTimeout(r, 125));
	}
	throw new Error(`server not healthy: ${String(last)}`);
}

interface SpawnOpts {
	hubMode?: boolean;
	hubURL?: string;
	allowPlainHTTP?: boolean;
	bindAddr?: string;
	selfURL?: string;
	nodeName?: string;
	// When set, reuse this directory rather than copying the fixture again.
	// Required when restarting a spoke so the persisted identity_fingerprint
	// survives — otherwise the hub rejects the restart as a 409 duplicate
	// node_id.
	reuseRoot?: string;
}

async function spawnServer(opts: SpawnOpts): Promise<SpawnedServer> {
	const bin = ensureDdxBinary();
	const port = await freePort();
	const bindAddr = opts.bindAddr ?? '127.0.0.1';
	const root = opts.reuseRoot ?? copyFixture();
	const args = [
		'server',
		'--port',
		String(port),
		'--addr',
		bindAddr,
		'--tsnet=false'
	];
	if (opts.hubMode) args.push('--hub-mode');
	if (opts.allowPlainHTTP) args.push('--federation-allow-plain-http');
	if (opts.hubURL) args.push('--hub-address', opts.hubURL);
	if (opts.selfURL) args.push('--federation-self-url', opts.selfURL);
	const child = spawn(bin, args, {
		cwd: root,
		env: {
			...process.env,
			DDX_NODE_NAME: opts.nodeName ?? `fed-e2e-${port}`,
			XDG_DATA_HOME: path.join(root, '.xdg-data'),
			XDG_CONFIG_HOME: path.join(root, '.xdg-config')
		}
	});
	const s: SpawnedServer = {
		port,
		addr: bindAddr,
		baseURL: `https://127.0.0.1:${port}`,
		root,
		child,
		stdoutBuf: '',
		stderrBuf: '',
		api: await playwrightRequest.newContext({
			baseURL: `https://127.0.0.1:${port}`,
			ignoreHTTPSErrors: true
		})
	};
	child.stdout.on('data', (d) => {
		s.stdoutBuf += d.toString();
	});
	child.stderr.on('data', (d) => {
		s.stderrBuf += d.toString();
	});
	await waitHealthy(s.api, child);
	return s;
}

async function stopServer(s: SpawnedServer, opts: { keepRoot?: boolean } = {}) {
	await s.api.dispose().catch(() => {});
	if (s.child.exitCode === null) {
		s.child.kill();
		await Promise.race([
			new Promise((r) => s.child.once('exit', r)),
			new Promise((r) =>
				setTimeout(() => {
					if (s.child.exitCode === null) s.child.kill('SIGKILL');
					r(undefined);
				}, 2500)
			)
		]);
	}
	if (!opts.keepRoot) fs.rmSync(s.root, { recursive: true, force: true });
}

async function nodeIdOf(s: SpawnedServer): Promise<string> {
	const r = await s.api.post('/graphql', {
		data: { query: '{ nodeInfo { id } }' }
	});
	const body = (await r.json()) as { data: { nodeInfo: { id: string } } };
	return body.data.nodeInfo.id;
}

async function federationNodes(s: SpawnedServer): Promise<
	Array<{ nodeId: string; status: string; name: string }>
> {
	const r = await s.api.post('/graphql', {
		data: {
			query:
				'{ federationNodes { nodeId status name } }'
		}
	});
	const body = (await r.json()) as {
		data: { federationNodes: Array<{ nodeId: string; status: string; name: string }> };
	};
	return body.data.federationNodes;
}

async function triggerFanOut(s: SpawnedServer): Promise<void> {
	// federatedBeads runs the fan-out path; the result is irrelevant — we
	// only need the side effect of StatusUpdates landing in the registry.
	await s.api.post('/graphql', {
		data: { query: '{ federatedBeads { nodeId } }' }
	});
}

function firstNonLoopbackIPv4(): string | null {
	const ifaces = os.networkInterfaces();
	for (const list of Object.values(ifaces)) {
		if (!list) continue;
		for (const a of list) {
			if (a.family === 'IPv4' && !a.internal) {
				// Skip docker bridges / link-local — pick the first usable.
				if (a.address.startsWith('169.254.')) continue;
				return a.address;
			}
		}
	}
	return null;
}

// ─── Federation 2-node tests ───────────────────────────────────────────────

test.describe('federation 2-node e2e', () => {
	test.setTimeout(90_000);

	test('hub /federation lists both nodes; scope=federation merges data; offline+restart cycle', async ({
		page
	}) => {
		const hub = await spawnServer({ hubMode: true, nodeName: 'fed-e2e-hub' });
		// Hub registers itself as a spoke (hub_spoke) so /federation lists it.
		await registerHubSelfAsSpoke(hub);

		const spoke = await spawnServer({
			hubURL: hub.baseURL,
			selfURL: '', // default
			nodeName: 'fed-e2e-spoke'
		});

		try {
			// Wait for both registrations to land in the hub registry.
			await expect
				.poll(async () => (await federationNodes(hub)).length, { timeout: 10_000 })
				.toBe(2);

			const fedNodes = await federationNodes(hub);
			const ids = new Set(fedNodes.map((n) => n.nodeId));
			expect(ids.has(await nodeIdOf(hub))).toBe(true);
			expect(ids.has(await nodeIdOf(spoke))).toBe(true);
			for (const n of fedNodes) {
				expect(['active', 'registered']).toContain(n.status);
			}

			// /federation page renders both nodes.
			await page.goto(`${hub.baseURL}/federation`);
			await expect(page.getByTestId('federation-page')).toBeVisible();
			const rows = page.getByTestId('federation-row');
			await expect(rows).toHaveCount(2);

			// Status badges reflect status (active OR registered before the
			// first heartbeat lands; both are healthy states).
			const badges = page.getByTestId('federation-status-badge');
			await expect(badges).toHaveCount(2);
			for (let i = 0; i < 2; i++) {
				const txt = (await badges.nth(i).textContent())?.trim().toLowerCase();
				expect(['active', 'registered']).toContain(txt);
			}

			// scope=federation fan-out combined view shows beads from BOTH
			// nodes. Each fixture workspace seeds 3 beads (fx-001/2/3); with
			// 2 federated nodes we expect 6 rows.
			const hubNodeId = await nodeIdOf(hub);
			await page.goto(`${hub.baseURL}/nodes/${hubNodeId}/beads?scope=federation`);
			await expect(page.getByTestId('scope-indicator')).toBeVisible();
			await expect(page.getByTestId('scope-toggle')).toContainText('federation');
			// Both fixture beads exist; expect at least one row per node by
			// their fixture title prefix.
			await expect.poll(
				async () => await page.getByText('Open ready bead').count(),
				{ timeout: 10_000 }
			).toBeGreaterThanOrEqual(2);

			// Toggle switches LOCAL vs FEDERATION.
			await page.getByTestId('scope-toggle').click();
			await expect(page).not.toHaveURL(/scope=federation/);
			await expect(page.getByTestId('scope-indicator')).toHaveCount(0);
			// Local view still has 3 beads (fx-001/2/3).
			await expect(page.getByText('Open ready bead')).toBeVisible();

			// Stop the spoke process. Keep its root so the persisted
			// identity_fingerprint survives the restart cycle (re-registration
			// with a different fingerprint would trip the hub's 409 duplicate-
			// node_id guard).
			const spokeRoot = spoke.root;
			await stopServer(spoke, { keepRoot: true });
			// Close the hubAsSpoke too if it points at the spoke — it does
			// not, the hub registers itself; keep it running.

			await expect
				.poll(
					async () => {
						await triggerFanOut(hub);
						const ns = await federationNodes(hub);
						const found = ns.find((n) => n.name === 'fed-e2e-spoke');
						return found?.status ?? 'missing';
					},
					{ timeout: 15_000, intervals: [500, 1000, 1500] }
				)
				.toBe('offline');

			// /federation page reflects the offline badge for the spoke.
			await page.goto(`${hub.baseURL}/federation`);
			const offlineRow = page.locator('[data-testid="federation-row"][data-status="offline"]');
			await expect(offlineRow).toHaveCount(1);
			await expect(
				offlineRow.locator('[data-testid="federation-status-badge"]')
			).toContainText(/offline/i);

			// Restart the spoke — registration alone (handshake → StatusActive)
			// returns it to active without waiting on a heartbeat tick.
			const spoke2 = await spawnServer({
				hubURL: hub.baseURL,
				nodeName: 'fed-e2e-spoke',
				reuseRoot: spokeRoot
			});
			try {
				await expect
					.poll(
						async () => {
							const ns = await federationNodes(hub);
							const found = ns.find((n) => n.name === 'fed-e2e-spoke');
							return found?.status ?? 'missing';
						},
						{ timeout: 15_000, intervals: [500, 1000] }
					)
					.toMatch(/active|registered/);

				await page.goto(`${hub.baseURL}/federation`);
				const activeRows = page.locator(
					'[data-testid="federation-row"][data-status="active"], [data-testid="federation-row"][data-status="registered"]'
				);
				await expect(activeRows).toHaveCount(2);
			} finally {
				await stopServer(spoke2);
			}
		} finally {
			await stopServer(hub);
		}
	});
});

// Drive the hub to register itself as a spoke (hub_spoke role) via a direct
// loopback POST to /api/federation/register. This avoids spinning up a third
// process just to obtain a 2-row /federation listing.
async function registerHubSelfAsSpoke(hub: SpawnedServer): Promise<void> {
	const hubNodeId = await nodeIdOf(hub);
	const resp = await hub.api.post('/api/federation/register', {
		data: {
			node_id: hubNodeId,
			identity_fingerprint: 'hub-self-fp',
			name: 'fed-e2e-hub',
			url: hub.baseURL,
			ddx_version: '0.1.0',
			schema_version: '1',
			capabilities: ['beads', 'runs']
		}
	});
	if (!resp.ok()) throw new Error(`hub self-register failed: ${resp.status()}`);
}

// ─── ts-net guard tests ────────────────────────────────────────────────────

test.describe('federation ts-net guard', () => {
	test.setTimeout(60_000);

	test('plain-HTTP non-loopback registration is rejected without --federation-allow-plain-http', async () => {
		const externalIP = firstNonLoopbackIPv4();
		test.skip(!externalIP, 'no non-loopback IPv4 interface available');

		const hub = await spawnServer({
			hubMode: true,
			bindAddr: '0.0.0.0',
			nodeName: 'tsnet-guard-strict'
		});
		try {
			const externalAPI = await playwrightRequest.newContext({
				baseURL: `https://${externalIP}:${hub.port}`,
				ignoreHTTPSErrors: true
			});
			try {
				const resp = await externalAPI.post('/api/federation/register', {
					data: {
						node_id: 'attacker-node',
						identity_fingerprint: 'fp',
						name: 'attacker',
						url: 'https://attacker.example:7743',
						ddx_version: '0.1.0',
						schema_version: '1',
						capabilities: ['beads']
					}
				});
				expect(resp.status()).toBe(403);
				const body = (await resp.json()) as { error?: string };
				expect(body.error ?? '').toMatch(/federation-allow-plain-http|trusted|loopback|forbidden/i);
			} finally {
				await externalAPI.dispose();
			}

			// Sanity: registry must NOT have recorded the attacker node.
			const fedNodes = await federationNodes(hub);
			expect(fedNodes.find((n) => n.nodeId === 'attacker-node')).toBeUndefined();
		} finally {
			await stopServer(hub);
		}
	});

	test('plain-HTTP non-loopback registration is accepted with --federation-allow-plain-http and emits WARN', async () => {
		const externalIP = firstNonLoopbackIPv4();
		test.skip(!externalIP, 'no non-loopback IPv4 interface available');

		const hub = await spawnServer({
			hubMode: true,
			allowPlainHTTP: true,
			bindAddr: '0.0.0.0',
			nodeName: 'tsnet-guard-permissive'
		});
		try {
			const externalAPI = await playwrightRequest.newContext({
				baseURL: `https://${externalIP}:${hub.port}`,
				ignoreHTTPSErrors: true
			});
			try {
				const resp = await externalAPI.post('/api/federation/register', {
					data: {
						node_id: 'opt-out-node',
						identity_fingerprint: 'fp',
						name: 'opt-out',
						url: 'https://opt-out.example:7743',
						ddx_version: '0.1.0',
						schema_version: '1',
						capabilities: ['beads']
					}
				});
				expect(resp.status()).toBe(200);
			} finally {
				await externalAPI.dispose();
			}

			// Registry now contains the registered node.
			await expect
				.poll(
					async () => (await federationNodes(hub)).find((n) => n.nodeId === 'opt-out-node')?.nodeId,
					{ timeout: 5_000 }
				)
				.toBe('opt-out-node');

			// WARN log captured: federation_hub.go writes
			//   "WARN: federation: accepted plain-HTTP registration node_id=..."
			// to stderr via log.Printf.
			expect(hub.stderrBuf).toMatch(/accepted plain-HTTP registration/i);
			expect(hub.stderrBuf).toMatch(/opt-out-node/);
		} finally {
			await stopServer(hub);
		}
	});
});
