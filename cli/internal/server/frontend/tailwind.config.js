import designTokens from './design-tokens.json' with { type: 'json' };

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,svelte,jsx,tsx}'],
  theme: {
    extend: {
      ...designTokens.theme.extend,
    },
  },
  plugins: [],
};
