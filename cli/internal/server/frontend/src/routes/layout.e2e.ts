import { expect, test, type Page } from '@playwright/test';

const NODE_INFO = { id: 'node-abc', name: 'Test Node' };

async function mockGraphQL(page: Page) {
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
				body: JSON.stringify({ data: { projects: { edges: [] } } })
			});
		} else {
			await route.continue();
		}
	});
}

test('loads / and NavShell links exist', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');

	// NavShell brand
	await expect(page.getByText('DDx')).toBeVisible();

	// Sidebar nav links (rendered as spans since no project selected)
	const nav = page.locator('nav');
	for (const label of ['Beads', 'Documents', 'Graph', 'Workers', 'Sessions', 'Personas', 'Commits']) {
		await expect(nav.getByText(label)).toBeVisible();
	}
});

test('dark mode toggle updates html class', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/');

	const html = page.locator('html');
	const toggle = page.getByRole('button', { name: /toggle dark mode/i });
	await expect(toggle).toBeVisible();

	// Read initial class state
	const initialClass = await html.getAttribute('class') ?? '';
	const wasDark = initialClass.includes('dark');

	// Toggle once — class should flip
	await toggle.click();
	if (wasDark) {
		await expect(html).not.toHaveClass(/dark/);
	} else {
		await expect(html).toHaveClass(/dark/);
	}

	// Toggle again — class should revert
	await toggle.click();
	if (wasDark) {
		await expect(html).toHaveClass(/dark/);
	} else {
		await expect(html).not.toHaveClass(/dark/);
	}
});

test('bits-ui Button renders on /demo/ui-primitives', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/demo/ui-primitives');

	const button = page.getByRole('button', { name: 'bits-ui Button' });
	await expect(button).toBeVisible();
});

test('bits-ui Button has correct role attribute', async ({ page }) => {
	await mockGraphQL(page);
	await page.goto('/demo/ui-primitives');

	const button = page.getByRole('button', { name: 'bits-ui Button' });
	await expect(button).toBeVisible();

	// bits-ui Button.Root renders with data-button-root attribute
	await expect(button).toHaveAttribute('data-button-root', 'true');
});
