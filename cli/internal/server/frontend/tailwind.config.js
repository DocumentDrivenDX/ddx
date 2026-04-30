import designTokens from './design-tokens.json' with { type: 'json' };

const exported = designTokens.theme?.extend ?? {};
const colors = exported.colors ?? {};
const fontFamily = exported.fontFamily ?? {};
const fontSize = exported.fontSize ?? {};

const semanticColors = {
	'accent-lever': '#3B5B7A',
	'accent-fulcrum': colors.secondary,
	'accent-load': colors.tertiary,
	'dark-accent-lever': '#7BA3CC',
	'dark-accent-fulcrum': '#9CA0A8',
	'dark-accent-load': '#D4A53D',

	'bg-canvas': '#F4EFE6',
	'bg-surface': colors.surface,
	'bg-elevated': colors.background,
	'dark-bg-canvas': '#1A1815',
	'dark-bg-surface': '#26231F',
	'dark-bg-elevated': '#2E2A25',

	'fg-ink': colors.text,
	'fg-muted': '#4B5563',
	'dark-fg-ink': '#EDE6D6',
	'dark-fg-muted': '#B8AF9C',

	'border-line': '#E4DDD0',
	'dark-border-line': '#34302A',

	'terminal-bg': colors['terminal-bg'],
	'terminal-bg-deep': '#0F0E0C',
	'terminal-fg': colors['terminal-fg'],

	error: '#BA1A1A',
	'dark-error': '#FFB4AB',
};

const semanticFonts = {
	'headline-lg': fontFamily.h1,
	'headline-md': fontFamily.h2,
	'body-md': fontFamily['body-md'],
	'body-sm': fontFamily['body-sm'],
	'label-caps': fontFamily.label,
	'mono-code': ['ui-monospace', 'SFMono-Regular', ...(fontFamily.code ?? []), 'monospace'],
};

const semanticFontSize = {
	'headline-lg': ['20px', { fontWeight: '800', lineHeight: '1.2', letterSpacing: '0' }],
	'headline-md': ['16px', { fontWeight: '600', lineHeight: '1.25', letterSpacing: '0' }],
	'body-md': ['14px', { fontWeight: '400', lineHeight: '1.5', letterSpacing: '0' }],
	'body-sm': ['13px', { fontWeight: '400', lineHeight: '1.4', letterSpacing: '0' }],
	'label-caps': ['11px', { fontWeight: '700', lineHeight: '1', letterSpacing: '0.05em' }],
	'mono-code': ['13px', { fontWeight: '400', lineHeight: '1.4', letterSpacing: '0' }],
};

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,svelte,jsx,tsx}'],
  theme: {
    extend: {
      ...exported,
      colors: {
        ...colors,
        ...semanticColors,
      },
      fontFamily: {
        ...fontFamily,
        ...semanticFonts,
      },
      fontSize: {
        ...fontSize,
        ...semanticFontSize,
      },
    },
  },
  plugins: [],
};
