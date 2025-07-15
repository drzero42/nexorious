import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
	plugins: [
		svelte({
			hot: !process.env.VITEST
		})
	],
	test: {
		include: ['src/**/*.{test,spec}.{js,ts}'],
		environment: 'jsdom',
		setupFiles: ['src/test-utils/setup.ts', 'src/test-utils/auth-mocks.ts', 'src/test-utils/navigation-mocks.ts'],
		coverage: {
			provider: 'v8',
			reporter: ['text', 'json', 'html'],
			exclude: [
				'node_modules/',
				'src/test-utils/',
				'**/*.d.ts',
				'**/*.config.*',
				'src/app.html'
			],
			thresholds: {
				lines: 70,
				functions: 70,
				branches: 70,
				statements: 70
			}
		},
		globals: true
	},
	resolve: {
		alias: {
			$lib: new URL('./src/lib', import.meta.url).pathname,
			'$app/environment': new URL('./src/test-utils/app-environment.js', import.meta.url).pathname,
			'$app/navigation': new URL('./src/test-utils/navigation-mocks.ts', import.meta.url).pathname,
			'$app/stores': new URL('./src/test-utils/navigation-mocks.ts', import.meta.url).pathname,
			'$env/dynamic/public': new URL('./src/test-utils/env-mock.js', import.meta.url).pathname
		}
	}
});