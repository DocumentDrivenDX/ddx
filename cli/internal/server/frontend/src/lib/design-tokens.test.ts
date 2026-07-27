import { describe, it, expect } from 'vitest';
import designTokens from '../../design-tokens.json';
import tailwindConfig from '../../tailwind.config.js';

/** Banned generic Stitch export placeholders (case-insensitive). */
const GENERIC_PALETTE = [
	'#4878c6',
	'#35a35f',
	'#8e35a3',
	'#f9fafb',
	'#0f1117',
	'#e8eaf0',
	'#111827'
];

/**
 * Flatten merged theme.extend.colors the same way tailwind.config.js does:
 * spread of exported colors + semanticColors (see tailwind.config.js:63-66).
 */
function mergedColors(): Record<string, unknown> {
	const theme = tailwindConfig.theme?.extend as { colors?: Record<string, unknown> } | undefined;
	return { ...(theme?.colors ?? {}) };
}

function allColorStrings(colors: Record<string, unknown>): string[] {
	const out: string[] = [];
	for (const value of Object.values(colors)) {
		if (typeof value === 'string') {
			out.push(value);
		}
	}
	return out;
}

describe('design tokens', () => {
	it('TestDesignTokensCarryWarmPalette', () => {
		const colors = mergedColors();
		const values = allColorStrings(colors).map((v) => v.toLowerCase());
		for (const generic of GENERIC_PALETTE) {
			expect(values, `merged palette must not contain generic ${generic}`).not.toContain(
				generic.toLowerCase()
			);
		}
	});

	it('TestDesignTokensMatchDesignMd', () => {
		const colors = mergedColors();
		// Cross-checked against .stitch/DESIGN.md colors: block
		expect(colors['accent-lever']).toBe('#3B5B7A');
		expect(colors['accent-load']).toBe('#A8801F');
		expect(colors['accent-fulcrum']).toBe('#3F4147');
		expect(colors['bg-canvas']).toBe('#F4EFE6');
		expect(colors['bg-surface']).toBe('#FBF8F2');
		expect(colors['fg-ink']).toBe('#1F2125');
		expect(colors['fg-muted']).toBe('#6B6558');
		expect(colors['border-line']).toBe('#E4DDD0');
		expect(colors['terminal-bg']).toBe('#1F2125');
		expect(colors['terminal-fg']).toBe('#D8D2C4');
		expect(colors['dark-accent-lever']).toBe('#7BA3CC');
		expect(colors['dark-accent-load']).toBe('#D4A53D');
		expect(colors['dark-accent-fulcrum']).toBe('#9CA0A8');
	});

	it('TestSemanticAliasLayerRetained', () => {
		const tokenColors = designTokens.theme?.extend?.colors ?? {};
		// Keys that semanticColors in tailwind.config.js dereferences
		for (const key of [
			'secondary',
			'tertiary',
			'surface',
			'background',
			'text',
			'terminal-bg',
			'terminal-fg'
		] as const) {
			expect(tokenColors[key], `design-tokens.json must define colors.${key}`).toBeDefined();
		}

		const colors = mergedColors();
		const required = [
			'accent-lever',
			'accent-fulcrum',
			'accent-load',
			'bg-canvas',
			'bg-surface',
			'bg-elevated',
			'fg-ink',
			'fg-muted',
			'border-line',
			'terminal-bg',
			'terminal-fg',
			'error',
			'dark-accent-lever',
			'dark-accent-fulcrum',
			'dark-accent-load',
			'dark-bg-canvas',
			'dark-bg-surface',
			'dark-bg-elevated',
			'dark-fg-ink',
			'dark-fg-muted',
			'dark-border-line',
			'dark-error'
		] as const;

		for (const key of required) {
			expect(colors[key], `merged colors must define ${key}`).toBeDefined();
			expect(colors[key], `${key} must not be undefined`).not.toBeUndefined();
		}

		// Alias layer present: semantic class names still resolve (non-empty strings)
		for (const key of required) {
			expect(typeof colors[key]).toBe('string');
			expect(String(colors[key]).length).toBeGreaterThan(0);
		}
	});
});
