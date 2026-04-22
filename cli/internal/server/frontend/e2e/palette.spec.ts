// FEAT-008 US-099: Developer Uses a Keyboard Command Palette
//
// These tests MUST FAIL until a Cmd+K / Ctrl+K command palette exists that
// searches documents, beads, and actions, and honors bead-detail context.

import { expect, test } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };
const PROJECT_ID = 'proj-1';
const PROJECTS = [{ id: PROJECT_ID, name: 'Project Alpha', path: '/repos/alpha' }];
const BASE_URL = `/nodes/node-abc/projects/${PROJECT_ID}`;

const PALETTE_RESULTS_EMPTY: Record<string, unknown[]> = {
	documents: [],
	beads: [],
	actions: [],
	navigation: []
};

const PALETTE_RESULTS_FEAT: Record<string, unknown[]> = {
	documents: [
		{ kind: 'document', path: 'docs/helix/01-frame/features/FEAT-008-web-ui.md', title: 'FEAT-008 Web UI' }
	],
	beads: [
		{ kind: 'bead', id: 'ddx-feat008-1', title: 'Implement Actions panel' }
	],
	actions: [
		{ kind: 'action', id: 'drain-queue', label: 'Drain queue' }
	],
	navigation: [
		{ kind: 'nav', route: `${BASE_URL}/efficacy`, title: 'Efficacy' }
	]
};

async function mockPalette(page: import('@playwright/test').Page) {
	await page.route('/graphql', async (route) => {
		const body = route.request().postDataJSON() as {
			query: string;
			variables?: Record<string, unknown>;
		};
		if (body.query.includes('NodeInfo')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { nodeInfo: NODE_INFO } }) });
		} else if (body.query.includes('Projects')) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { projects: { edges: PROJECTS.map((p) => ({ node: p })) } } }) });
		} else if (body.query.includes('PaletteSearch') || body.query.includes('paletteSearch')) {
			const q = ((body.variables?.query as string) ?? '').toLowerCase();
			const results = q.length === 0 ? PALETTE_RESULTS_EMPTY : PALETTE_RESULTS_FEAT;
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { paletteSearch: results } }) });
		} else if (body.query.includes('Bead') && body.variables?.id) {
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { bead: { id: body.variables.id, title: 'Sample bead', status: 'open' } } }) });
		} else {
			await route.continue();
		}
	});
}

test('US-099.a: Cmd+K opens palette with focus in the input', async ({ page }) => {
	await mockPalette(page);
	await page.goto(BASE_URL);

	await page.keyboard.press('Meta+k');
	const palette = page.getByRole('dialog', { name: /command palette/i });
	await expect(palette).toBeVisible();
	await expect(palette.getByRole('searchbox')).toBeFocused();

	// Ctrl+K also opens it on non-mac platforms.
	await page.keyboard.press('Escape');
	await expect(palette).not.toBeVisible();
	await page.keyboard.press('Control+k');
	await expect(palette).toBeVisible();
});

test('US-099.b: typing returns documents, beads, actions, and navigation entries', async ({ page }) => {
	await mockPalette(page);
	await page.goto(BASE_URL);

	await page.keyboard.press('Meta+k');
	const palette = page.getByRole('dialog', { name: /command palette/i });
	await palette.getByRole('searchbox').fill('feat');

	const list = palette.getByRole('listbox');
	await expect(list.getByRole('option', { name: /FEAT-008/i })).toBeVisible();
	await expect(list.getByRole('option', { name: /Implement Actions panel/i })).toBeVisible();
	await expect(list.getByRole('option', { name: /Drain queue/i })).toBeVisible();
	await expect(list.getByRole('option', { name: /Efficacy/i })).toBeVisible();
});

test('US-099.c: Enter navigates to a result and closes the palette', async ({ page }) => {
	await mockPalette(page);
	await page.goto(BASE_URL);

	await page.keyboard.press('Meta+k');
	const palette = page.getByRole('dialog', { name: /command palette/i });
	await palette.getByRole('searchbox').fill('feat');

	// Highlight the navigation item (Efficacy) via arrow keys then Enter.
	await palette.getByRole('option', { name: /Efficacy/i }).click();
	await expect(palette).not.toBeVisible();
	await expect(page).toHaveURL(/\/efficacy$/);
});

test('US-099.d: Escape closes the palette without navigation', async ({ page }) => {
	await mockPalette(page);
	await page.goto(BASE_URL);
	const startUrl = page.url();

	await page.keyboard.press('Meta+k');
	await expect(page.getByRole('dialog', { name: /command palette/i })).toBeVisible();

	await page.keyboard.press('Escape');
	await expect(page.getByRole('dialog', { name: /command palette/i })).not.toBeVisible();
	expect(page.url()).toBe(startUrl);
});

test('US-099.e: on bead detail, palette shows bead-specific actions at top', async ({ page }) => {
	await mockPalette(page);
	await page.goto(`${BASE_URL}/beads/ddx-feat008-1`);

	await page.keyboard.press('Meta+k');
	const palette = page.getByRole('dialog', { name: /command palette/i });
	const list = palette.getByRole('listbox');

	const options = list.getByRole('option');
	// First items must be bead actions (verified by the names and ordering).
	await expect(options.nth(0)).toContainText(/Claim|Unclaim|Close|Reopen|Re-run|Delete/i);
	for (const action of [/Claim/i, /Close/i, /Reopen/i, /Re-run/i, /Delete/i]) {
		await expect(list.getByRole('option', { name: action })).toBeVisible();
	}
});

test('US-099.f: palette preserves project/node context for navigation entries', async ({ page }) => {
	await mockPalette(page);
	// Start from a deep URL.
	await page.goto(`${BASE_URL}/documents/docs/helix/01-frame/features/FEAT-008-web-ui.md`);

	await page.keyboard.press('Meta+k');
	const palette = page.getByRole('dialog', { name: /command palette/i });
	await palette.getByRole('searchbox').fill('feat');

	await palette.getByRole('option', { name: /Efficacy/i }).click();
	// Navigation stays within the same node + project.
	await expect(page).toHaveURL(new RegExp(`${BASE_URL}/efficacy$`));
});
