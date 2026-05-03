import { expect, test } from '@playwright/test';

// Layout/shell expectations driven by the live Go-server fixture harness.
// The harness boots ddx-server from a temp `ddx-e2e-XXXXXX` workspace so
// nodeInfo and the project list resolve against seeded fixture data instead
// of the developer's $HOME or the repo's live `.ddx/` state.

test('loads / and NavShell links exist', async ({ page }) => {
	await page.goto('/');
	await page.waitForSelector('nav');

	// NavShell brand
	await expect(page.getByRole('link', { name: 'DDx' })).toBeVisible();

	// No project is auto-selected from the fixture's project list, so the
	// project-scoped sidebar entries render as plain text (spans) and remain
	// matchable via getByText.
	const nav = page.locator('nav');
	for (const label of [
		'Beads',
		'Documents',
		'Graph',
		'Workers',
		'Personas',
		'Commits',
		'All Beads'
	]) {
		await expect(nav.getByText(label, { exact: true })).toBeVisible();
	}
});

test('dark mode toggle updates html class', async ({ page }) => {
	await page.goto('/');

	const html = page.locator('html');
	const toggle = page.getByRole('button', { name: /toggle dark mode/i });
	await expect(toggle).toBeVisible();

	// Read initial class state
	const initialClass = (await html.getAttribute('class')) ?? '';
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
	await page.goto('/demo/ui-primitives');

	const button = page.getByRole('button', { name: 'bits-ui Button' });
	await expect(button).toBeVisible();
});

test('bits-ui Button has correct role attribute', async ({ page }) => {
	await page.goto('/demo/ui-primitives');

	const button = page.getByRole('button', { name: 'bits-ui Button' });
	await expect(button).toBeVisible();

	// bits-ui Button.Root renders with data-button-root attribute
	await expect(button).toHaveAttribute('data-button-root', 'true');
});
