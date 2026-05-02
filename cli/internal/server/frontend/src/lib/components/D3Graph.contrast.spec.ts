import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const graphSource = readFileSync(resolve(here, './D3Graph.svelte'), 'utf8');
const cssSource = readFileSync(resolve(here, '../../app.css'), 'utf8');

// Canvas backgrounds the graph paints onto. Mirrored from tailwind.config.js
// semanticColors — if those move, update both sides.
const tokens = {
	'bg-canvas': '#F4EFE6',
	'dark-bg-canvas': '#1A1815',
	'border-line': '#E4DDD0',
	'dark-border-line': '#34302A'
};

function srgbChannel(c: number): number {
	const v = c / 255;
	return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
}

function relLuminance(hex: string): number {
	const m = hex.replace('#', '');
	const r = parseInt(m.slice(0, 2), 16);
	const g = parseInt(m.slice(2, 4), 16);
	const b = parseInt(m.slice(4, 6), 16);
	return 0.2126 * srgbChannel(r) + 0.7152 * srgbChannel(g) + 0.0722 * srgbChannel(b);
}

function contrast(a: string, b: string): number {
	const la = relLuminance(a);
	const lb = relLuminance(b);
	const [hi, lo] = la > lb ? [la, lb] : [lb, la];
	return (hi + 0.05) / (lo + 0.05);
}

function readVar(scope: 'light' | 'dark', name: string): string {
	// CSS is split into a light block (":root, :root.light, .light") and a dark
	// block (":root.dark, .dark"). Grab the body for the requested scope and
	// pull the variable definition out of it.
	const blockRe =
		scope === 'dark'
			? /:root\.dark,\s*\n?\s*\.dark\s*\{([\s\S]*?)\}/
			: /:root,\s*\n?\s*:root\.light,\s*\n?\s*\.light\s*\{([\s\S]*?)\}/;
	const block = blockRe.exec(cssSource);
	if (!block) throw new Error(`could not locate ${scope} block in app.css`);
	const m = new RegExp(`--${name}\\s*:\\s*(#[0-9A-Fa-f]{6})`).exec(block[1]);
	if (!m) throw new Error(`--${name} not found in ${scope} block`);
	return m[1];
}

describe('D3Graph edge ink', () => {
	it('paints edges and arrowheads via graph-edge / graph-edge-arrow CSS vars (no border-line, no opacity multiplier)', () => {
		expect(graphSource).toContain("'graph-edge'");
		expect(graphSource).toContain("'graph-edge-arrow'");
		expect(graphSource).not.toMatch(/stroke-border-line|fill-border-line/);
		// Subtlety must be carried by color, not alpha — no stroke-opacity attr.
		expect(graphSource).not.toMatch(/stroke-opacity/);
	});

	it('--graph-edge resolves to a value with WCAG non-text 3:1 against canvas in both themes', () => {
		const lightEdge = readVar('light', 'graph-edge');
		const darkEdge = readVar('dark', 'graph-edge');
		expect(contrast(lightEdge, tokens['bg-canvas'])).toBeGreaterThanOrEqual(3);
		expect(contrast(darkEdge, tokens['dark-bg-canvas'])).toBeGreaterThanOrEqual(3);
	});

	it('proves the previous border-line tokens were below the 3:1 threshold (regression guard)', () => {
		const lightOld = contrast(tokens['border-line'], tokens['bg-canvas']);
		const darkOld = contrast(tokens['dark-border-line'], tokens['dark-bg-canvas']);
		expect(lightOld).toBeLessThan(3);
		expect(darkOld).toBeLessThan(3);
	});
});
