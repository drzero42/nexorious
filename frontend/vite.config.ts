import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	build: {
		target: 'esnext',
		cssCodeSplit: true,
		assetsInlineLimit: 4096,
		chunkSizeWarningLimit: 500,
		reportCompressedSize: true,
		sourcemap: process.env.NODE_ENV === 'development',
		rollupOptions: {
			output: {
				manualChunks: {
					vendor: ['svelte']
				},
				chunkFileNames: 'assets/[name]-[hash].js',
				entryFileNames: 'assets/[name]-[hash].js',
				assetFileNames: 'assets/[name]-[hash].[ext]'
			}
		},
		minify: 'esbuild',
		cssMinify: true
	},
	plugins: [
		sveltekit()
	]
});
