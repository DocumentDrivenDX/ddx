// FEAT-008 US-098: Operator Browses and Installs Plugins
//
// These tests MUST FAIL until the Plugins page lists registry entries,
// install/uninstall/update actions are wired to server-side workers, and
// the plugin detail view exposes manifest/skills/prompts/templates.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}/plugins`;

const PLUGINS = [
	{
		name: 'helix',
		version: '1.4.2',
		installedVersion: '1.4.2',
		type: 'workflow',
		description: 'HELIX methodology: phases, gates, supervisory dispatch',
		keywords: ['workflow', 'methodology'],
		status: 'installed',
		registrySource: 'builtin',
		diskBytes: 4_200_000,
		manifest: { name: 'helix', version: '1.4.2' },
		skills: ['helix-align', 'helix-plan'],
		prompts: ['drain-queue', 'run-checks'],
		templates: ['FEAT-spec']
	},
	{
		name: 'frontend-design',
		version: '0.3.1',
		installedVersion: null,
		type: 'persona-pack',
		description: 'Palette-disciplined UI/UX review skill',
		keywords: ['design', 'ui', 'a11y'],
		status: 'available',
		registrySource: 'builtin',
		diskBytes: 800_000
	},
	{
		name: 'ddx-cost-tier',
		version: '0.5.0',
		installedVersion: '0.4.2',
		type: 'plugin',
		description: 'Cost-tiered routing policies for ddx agent',
		keywords: ['routing', 'cost'],
		status: 'update-available',
		registrySource: 'https://github.com/example/ddx-plugins',
		diskBytes: 1_200_000
	}
];

type PluginFixture = (typeof PLUGINS)[number];

async function mockPlugins(
	page: import('@playwright/test').Page,
	opts: {
		dispatchFn?: (req: Record<string, unknown>) => Record<string, unknown>;
		plugins?: PluginFixture[];
		pluginsListFn?: () => PluginFixture[];
	} = {}
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		const plugins = opts.pluginsListFn?.() ?? opts.plugins ?? PLUGINS;
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
				body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } })
			});
		} else if (body.query.includes('PluginsList') || body.query.includes('pluginsList')) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { pluginsList: plugins } })
			});
		} else if (body.query.includes('PluginDetail') || body.query.includes('pluginDetail')) {
			const name = (body.variables?.name as string) ?? 'helix';
			const p = plugins.find((x) => x.name === name) ?? plugins[0];
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { pluginDetail: p } })
			});
		} else if (body.query.includes('PluginDispatch') || body.query.includes('pluginDispatch')) {
			const result = opts.dispatchFn
				? opts.dispatchFn(body.variables ?? {})
				: { id: 'worker-install-1', state: 'queued', action: body.variables?.action };
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ data: { pluginDispatch: result } })
			});
		} else {
			await route.continue();
		}
	});
}

async function mockWorkerProgress(page: import('@playwright/test').Page) {
	const subscriptions = new Map<
		string,
		Array<{ id: string; ws: { send: (message: string) => void } }>
	>();

	await page.routeWebSocket('**/graphql', (ws) => {
		ws.onMessage((msg) => {
			const parsed = JSON.parse(msg as string) as {
				id?: string;
				type: string;
				payload?: { query?: string; variables?: { workerID?: string } };
			};

			if (parsed.type === 'connection_init') {
				ws.send(JSON.stringify({ type: 'connection_ack' }));
				return;
			}

			if (
				parsed.type === 'subscribe' &&
				parsed.id &&
				parsed.payload?.query?.includes('workerProgress')
			) {
				const workerID = parsed.payload.variables?.workerID;
				if (!workerID) return;
				const existing = subscriptions.get(workerID) ?? [];
				existing.push({ id: parsed.id, ws });
				subscriptions.set(workerID, existing);
			}
		});
	});

	return {
		hasSubscription(workerID: string) {
			return (subscriptions.get(workerID)?.length ?? 0) > 0;
		},
		send(workerID: string, phase: string) {
			const entries = subscriptions.get(workerID) ?? [];
			for (const entry of entries) {
				entry.ws.send(
					JSON.stringify({
						id: entry.id,
						type: 'next',
						payload: {
							data: {
								workerProgress: {
									eventID: `${workerID}-${phase}`,
									workerID,
									phase,
									timestamp: '2026-04-22T12:00:00Z',
									logLine: null,
									beadID: null
								}
							}
						}
					})
				);
				entry.ws.send(JSON.stringify({ id: entry.id, type: 'complete' }));
			}
		}
	};
}

test('US-098.a: plugins page lists every registry entry with version, type, status', async ({
	page
}) => {
	await mockPlugins(page);
	await page.goto(BASE_URL);

	for (const p of PLUGINS) {
		const card = page.getByRole('article', { name: new RegExp(p.name, 'i') });
		await expect(card).toBeVisible();
		await expect(card).toContainText(p.version);
		await expect(card).toContainText(p.type);
		await expect(card).toContainText(p.description);
	}

	// Status badges.
	await expect(page.getByRole('article', { name: /helix/i }).getByText(/installed/i)).toBeVisible();
	await expect(
		page.getByRole('article', { name: /frontend-design/i }).getByText(/available/i)
	).toBeVisible();
	await expect(
		page.getByRole('article', { name: /ddx-cost-tier/i }).getByText(/update/i)
	).toBeVisible();
});

test('US-098.b: Install fires pluginDispatch mutation with scope + streams progress', async ({
	page
}) => {
	let captured: Record<string, unknown> | null = null;
	await mockPlugins(page, {
		dispatchFn: (req) => {
			captured = req;
			return { id: 'worker-install-42', state: 'queued', action: 'install' };
		}
	});

	await page.goto(BASE_URL);
	await page
		.getByRole('article', { name: /frontend-design/i })
		.getByRole('button', { name: /install/i })
		.click();

	const dialog = page.getByRole('dialog', { name: /install/i });
	await expect(dialog).toBeVisible();

	// Scope + disk space info.
	const scope = dialog.getByRole('radiogroup', { name: /scope/i });
	await expect(scope).toBeVisible();
	await expect(dialog).toContainText(/disk/i);
	await expect(dialog).toContainText(/800.*(kb|b)/i);

	await dialog.getByRole('radio', { name: /project/i }).check();
	await dialog.getByRole('button', { name: /confirm|install/i }).click();

	await expect.poll(() => captured).not.toBeNull();
	expect(captured).toMatchObject({ name: 'frontend-design', action: 'install', scope: 'project' });

	// Link to the streaming worker.
	await expect(page.getByRole('link', { name: /worker-install-42/ })).toBeVisible();
	await expect(page.getByRole('article', { name: /frontend-design/i })).toContainText(
		'Installing...'
	);
	await expect(
		page
			.getByRole('article', { name: /frontend-design/i })
			.getByRole('button', { name: /install/i })
	).toBeDisabled();
});

test('US-098.c: plugin detail shows manifest, skills, prompts, templates, Uninstall', async ({
	page
}) => {
	await mockPlugins(page);
	await page.goto(`${BASE_URL}/helix`);

	const manifest = page.getByRole('region', { name: /manifest/i });
	await expect(manifest).toContainText(/name:\s*helix/i);
	await expect(manifest).toContainText(/version:\s*1\.4\.2/);

	await expect(page.getByRole('region', { name: /skills/i })).toContainText('helix-align');
	await expect(page.getByRole('region', { name: /prompts/i })).toContainText('drain-queue');
	await expect(page.getByRole('region', { name: /templates/i })).toContainText('FEAT-spec');

	// Uninstall with confirmation.
	await page.getByRole('button', { name: /uninstall/i }).click();
	const confirm = page.getByRole('dialog', { name: /uninstall/i });
	await expect(confirm).toBeVisible();
	await expect(confirm.getByRole('button', { name: /confirm|remove/i })).toBeVisible();
	await expect(confirm.getByRole('button', { name: /cancel/i })).toBeVisible();
});

test('US-098.d: update-available card shows both versions and Update action', async ({ page }) => {
	let captured: Record<string, unknown> | null = null;
	await mockPlugins(page, {
		dispatchFn: (req) => {
			captured = req;
			return { id: 'worker-upd-1', state: 'queued', action: 'update' };
		}
	});
	await page.goto(BASE_URL);

	const card = page.getByRole('article', { name: /ddx-cost-tier/i });
	await expect(card).toContainText('0.4.2');
	await expect(card).toContainText('0.5.0');

	await card.getByRole('button', { name: /update/i }).click();
	await expect.poll(() => captured).not.toBeNull();
	expect(captured).toMatchObject({ name: 'ddx-cost-tier', action: 'update' });
});

test('plugin update refreshes card when worker completes successfully', async ({ page }) => {
	const stream = await mockWorkerProgress(page);
	let plugins = [...PLUGINS];

	await mockPlugins(page, {
		pluginsListFn: () => plugins,
		dispatchFn: () => ({ id: 'worker-upd-success', state: 'queued', action: 'update' })
	});

	await page.goto(BASE_URL);
	const card = page.getByRole('article', { name: /ddx-cost-tier/i });
	await card.getByRole('button', { name: /update/i }).click();

	await expect(card).toContainText('Updating...');
	await expect(card.getByRole('button', { name: /update/i })).toBeDisabled();
	await expect(page.getByRole('link', { name: /ddx-cost-tier.*worker-upd-success/ })).toBeVisible();
	await expect.poll(() => stream.hasSubscription('worker-upd-success')).toBe(true);

	plugins = plugins.map((plugin) =>
		plugin.name === 'ddx-cost-tier'
			? { ...plugin, status: 'installed', installedVersion: plugin.version }
			: plugin
	);
	stream.send('worker-upd-success', 'done');

	await expect(card).toContainText(/installed/i, { timeout: 2000 });
	await expect(card).toContainText('0.5.0');
	await expect(card.getByRole('button', { name: /update/i })).toHaveCount(0);
});

test('plugin update failure shows per-card failure without changing status', async ({ page }) => {
	const stream = await mockWorkerProgress(page);
	const plugins = [...PLUGINS];

	await mockPlugins(page, {
		pluginsListFn: () => plugins,
		dispatchFn: () => ({ id: 'worker-upd-failed', state: 'queued', action: 'update' })
	});

	await page.goto(BASE_URL);
	const card = page.getByRole('article', { name: /ddx-cost-tier/i });
	await card.getByRole('button', { name: /update/i }).click();
	await expect.poll(() => stream.hasSubscription('worker-upd-failed')).toBe(true);
	stream.send('worker-upd-failed', 'failed');

	await expect(card.getByRole('link', { name: /update failed/i })).toBeVisible({ timeout: 2000 });
	await expect(card.getByRole('link', { name: /update failed/i })).toHaveAttribute(
		'title',
		/Update failed/
	);
	await expect(card.getByText(/update available/i)).toBeVisible();
	await expect(card).toContainText('0.4.2');
	await expect(card).toContainText('0.5.0');
	await expect(card.getByRole('button', { name: /update/i })).toBeEnabled();
});

test('plugin update falls back to polling when worker stream drops terminal event', async ({
	page
}) => {
	await page.addInitScript(() => {
		(window as typeof window & { __ddxPluginFallbackDelayMs?: number }).__ddxPluginFallbackDelayMs =
			100;
		(window as typeof window & { __ddxPluginPollIntervalMs?: number }).__ddxPluginPollIntervalMs =
			100;
	});
	await mockWorkerProgress(page);

	let plugins = [...PLUGINS];
	await mockPlugins(page, {
		pluginsListFn: () => plugins,
		dispatchFn: () => ({ id: 'worker-upd-dropout', state: 'queued', action: 'update' })
	});

	await page.goto(BASE_URL);
	const card = page.getByRole('article', { name: /ddx-cost-tier/i });
	await card.getByRole('button', { name: /update/i }).click();
	await expect(card).toContainText('Updating...');

	plugins = plugins.map((plugin) =>
		plugin.name === 'ddx-cost-tier'
			? { ...plugin, status: 'installed', installedVersion: plugin.version }
			: plugin
	);

	await expect(card).toContainText(/installed/i, { timeout: 2000 });
	await expect(page.getByRole('link', { name: /worker-upd-dropout/ })).toHaveCount(0);
});

test('two plugin updates track separate workers concurrently', async ({ page }) => {
	const stream = await mockWorkerProgress(page);
	const secondOutdated = {
		...PLUGINS[0],
		name: 'helix',
		version: '1.5.0',
		installedVersion: '1.4.2',
		status: 'update-available'
	};
	let plugins: PluginFixture[] = [secondOutdated, PLUGINS[2]];

	await mockPlugins(page, {
		pluginsListFn: () => plugins,
		dispatchFn: (req) => ({
			id: `worker-${req.name as string}`,
			state: 'queued',
			action: 'update'
		})
	});

	await page.goto(BASE_URL);
	const helix = page.getByRole('article', { name: /helix/i });
	const costTier = page.getByRole('article', { name: /ddx-cost-tier/i });

	await helix.getByRole('button', { name: /update/i }).click();
	await costTier.getByRole('button', { name: /update/i }).click();

	await expect(page.getByRole('link', { name: /helix.*worker-helix/ })).toBeVisible();
	await expect(
		page.getByRole('link', { name: /ddx-cost-tier.*worker-ddx-cost-tier/ })
	).toBeVisible();
	await expect(helix).toContainText('Updating...');
	await expect(costTier).toContainText('Updating...');

	await expect.poll(() => stream.hasSubscription('worker-helix')).toBe(true);
	await expect.poll(() => stream.hasSubscription('worker-ddx-cost-tier')).toBe(true);

	plugins = plugins.map((plugin) =>
		plugin.name === 'helix'
			? { ...plugin, status: 'installed', installedVersion: plugin.version }
			: plugin
	);
	stream.send('worker-helix', 'done');

	await expect(helix).toContainText(/installed/i, { timeout: 2000 });
	await expect(costTier).toContainText('Updating...');
	await expect(
		page.getByRole('link', { name: /ddx-cost-tier.*worker-ddx-cost-tier/ })
	).toBeVisible();
});
