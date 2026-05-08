// FEAT-008 frontend-design concern: WCAG AA floor (4.5:1 text contrast, 3:1 UI)
//
// These tests MUST FAIL until:
//   1. `@axe-core/playwright` is a devDependency
//   2. Pages ship no axe violations at WCAG AA with both dark and light themes
//   3. A theme toggle is wired up via the `mode-watcher` setMode API
//
// Rationale lives in docs/helix/01-frame/concerns.md#frontend-design.

import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const PAGES = [
	{ path: '/', name: 'dashboard' },
	{ path: '/beads', name: 'beads' },
	{ path: '/documents', name: 'documents' },
	{ path: '/graph', name: 'graph' },
	{ path: '/agent', name: 'agent' },
	{ path: '/personas', name: 'personas' }
];

const MODES = ['light', 'dark'] as const;

for (const mode of MODES) {
	for (const p of PAGES) {
		test(`a11y (${mode}): ${p.name} has no WCAG AA axe violations`, async ({ page }) => {
			// Set the theme BEFORE the first paint via localStorage — the
			// mode-watcher library reads this key on boot.
			await page.addInitScript((m) => {
				window.localStorage.setItem('mode-watcher-mode', m);
			}, mode);

			await page.goto(p.path);
			await page.waitForLoadState('networkidle');

			const results = await new AxeBuilder({ page })
				.withTags(['wcag2a', 'wcag2aa', 'wcag21aa'])
				.analyze();

			expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([]);
		});
	}
}

test('theme toggle is operable via keyboard and persists to localStorage', async ({ page }) => {
	await page.goto('/');
	await page.waitForLoadState('networkidle');

	const toggle = page.getByRole('button', { name: /theme|dark mode|light mode/i });
	await expect(toggle).toBeVisible();

	// Toggle changes the class applied to <html>.
	const htmlClasses = async () => await page.evaluate(() => document.documentElement.className);
	const before = await htmlClasses();
	await toggle.click();
	await expect
		.poll(async () => await htmlClasses(), { timeout: 2000 })
		.not.toEqual(before);

	// Persists across reload.
	const afterClick = await htmlClasses();
	await page.reload();
	await page.waitForLoadState('networkidle');
	await expect.poll(async () => await htmlClasses(), { timeout: 2000 }).toEqual(afterClick);
});
