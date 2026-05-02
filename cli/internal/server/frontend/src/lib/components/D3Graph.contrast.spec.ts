import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const graphSource = readFileSync(resolve(here, './D3Graph.svelte'), 'utf8');

// Token values mirrored from tailwind.config.js semanticColors. If those move,
// update both sides — this test exists to lock the WCAG contrast of edge ink
// against the canvas backgrounds the graph paints onto.
const tokens = {
	'bg-canvas': '#F4EFE6',
	'dark-bg-canvas': '#1A1815',
	'fg-muted': '#4B5563',
	'dark-fg-muted': '#B8AF9C',
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

describe('D3Graph edge ink', () => {
	it('uses fg-muted tokens for edge stroke and arrowhead fill (not border-line)', () => {
		// Tailwind v4 only generates text-* utilities for these custom tokens, so
		// the edge/arrow drive their stroke and fill from currentColor instead of
		// stroke-*/fill-* utilities. The text-fg-muted class still anchors the
		// canonical color choice.
		expect(graphSource).toContain("'text-fg-muted dark:text-dark-fg-muted'");
		expect(graphSource).toMatch(/\.attr\(\s*'stroke'\s*,\s*'currentColor'\s*\)/);
		expect(graphSource).toMatch(/\.attr\(\s*'fill'\s*,\s*'currentColor'\s*\)/);
		expect(graphSource).not.toMatch(/stroke-border-line|fill-border-line/);
	});

	it('chosen edge tokens meet WCAG non-text 3:1 against canvas in light and dark mode', () => {
		const light = contrast(tokens['fg-muted'], tokens['bg-canvas']);
		const dark = contrast(tokens['dark-fg-muted'], tokens['dark-bg-canvas']);
		expect(light).toBeGreaterThanOrEqual(3);
		expect(dark).toBeGreaterThanOrEqual(3);
	});

	it('proves the previous border-line tokens were below the 3:1 threshold (regression guard)', () => {
		const lightOld = contrast(tokens['border-line'], tokens['bg-canvas']);
		const darkOld = contrast(tokens['dark-border-line'], tokens['dark-bg-canvas']);
		expect(lightOld).toBeLessThan(3);
		expect(darkOld).toBeLessThan(3);
	});
});
