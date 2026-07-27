import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it, expect } from 'vitest';
import tailwindConfig from '../../tailwind.config.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const appCssPath = resolve(__dirname, '../app.css');

const NON_MONO_ROLES = ['headline-lg', 'headline-md', 'body-md', 'body-sm', 'label-caps'] as const;

function fontStacks(): Record<string, string[]> {
	const theme = tailwindConfig.theme?.extend as
		| { fontFamily?: Record<string, string | string[]> }
		| undefined;
	const families = theme?.fontFamily ?? {};
	const out: Record<string, string[]> = {};
	for (const [key, value] of Object.entries(families)) {
		out[key] = Array.isArray(value) ? value : [value];
	}
	return out;
}

describe('Inter font loading', () => {
	it('TestInterFaceIsImported', () => {
		const css = readFileSync(appCssPath, 'utf8');
		const fontsourceImport = /@import\s+['"]@fontsource-variable\/inter['"]\s*;/;
		const tailwindImport = /@import\s+['"]tailwindcss['"]\s*;/;

		expect(css, 'app.css must import @fontsource-variable/inter').toMatch(fontsourceImport);

		const fontIdx = css.search(fontsourceImport);
		const twIdx = css.search(tailwindImport);
		expect(twIdx, 'app.css must import tailwindcss').toBeGreaterThanOrEqual(0);
		expect(fontIdx, 'fontsource import must appear before tailwindcss import').toBeLessThan(twIdx);
	});

	it('TestInterFamilyInFontStacks', () => {
		const stacks = fontStacks();

		for (const role of NON_MONO_ROLES) {
			const stack = stacks[role];
			expect(stack, `semanticFonts role ${role} must exist`).toBeDefined();
			expect(stack[0], `${role} first entry must be Inter Variable`).toBe('Inter Variable');
			expect(stack[stack.length - 1], `${role} must end with sans-serif`).toBe('sans-serif');
		}

		const mono = stacks['mono-code'];
		expect(mono, 'mono-code must exist').toBeDefined();
		expect(mono[0], 'mono-code must still start with ui-monospace').toBe('ui-monospace');
	});
});
