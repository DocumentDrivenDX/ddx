import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import { sveltekit } from '@sveltejs/kit/vite';
import houdini from 'houdini/vite';
import path from 'path';

export default defineConfig({
	plugins: [houdini(), tailwindcss(), sveltekit()],
	resolve: {
		alias: {
			$houdini: path.resolve(process.cwd(), '$houdini')
		}
	},
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});
