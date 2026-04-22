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

async function mockPlugins(
	page: import('@playwright/test').Page,
	opts: { dispatchFn?: (req: Record<string, unknown>) => Record<string, unknown> } = {}
) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('PluginsList') || body.query.includes('pluginsList')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { pluginsList: PLUGINS } })});
		} else if (body.query.includes('PluginDetail') || body.query.includes('pluginDetail')) {
			const name = (body.variables?.name as string) ?? 'helix';
			const p = PLUGINS.find((x) => x.name === name) ?? PLUGINS[0];
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { pluginDetail: p } }) });
		} else if (body.query.includes('PluginDispatch') || body.query.includes('pluginDispatch')) {
			const result = opts.dispatchFn
				? opts.dispatchFn(body.variables ?? {})
				: { id: 'worker-install-1', state: 'queued', action: body.variables?.action };
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { pluginDispatch: result } }) });
		} else {
			await route.continue();
		}
	});
}

test('US-098.a: plugins page lists every registry entry with version, type, status', async ({ page }) => {
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
	await expect(page.getByRole('article', { name: /frontend-design/i }).getByText(/available/i)).toBeVisible();
	await expect(page.getByRole('article', { name: /ddx-cost-tier/i }).getByText(/update/i)).toBeVisible();
});

test('US-098.b: Install fires pluginDispatch mutation with scope + streams progress', async ({ page }) => {
	let captured: Record<string, unknown> | null = null;
	await mockPlugins(page, {
		dispatchFn: (req) => {
			captured = req;
			return { id: 'worker-install-42', state: 'queued', action: 'install' };
		}
	});

	await page.goto(BASE_URL);
	await page.getByRole('article', { name: /frontend-design/i }).getByRole('button', { name: /install/i }).click();

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
});

test('US-098.c: plugin detail shows manifest, skills, prompts, templates, Uninstall', async ({ page }) => {
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
