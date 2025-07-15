import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';

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
					vendor: ['svelte'],
					ui: ['@tailwindcss/typography'],
					pwa: ['workbox-window']
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
		sveltekit(),
		SvelteKitPWA({
			registerType: 'autoUpdate',
			workbox: {
				globPatterns: ['**/*.{js,css,html,ico,png,svg,webp,woff,woff2}'],
				runtimeCaching: [
					{
						urlPattern: /^https:\/\/api\.igdb\.com\/.*/i,
						handler: 'CacheFirst',
						options: {
							cacheName: 'igdb-api-cache',
							expiration: {
								maxEntries: 100,
								maxAgeSeconds: 60 * 60 * 24 * 7 // 7 days
							}
						}
					},
					{
						urlPattern: /^https:\/\/images\.igdb\.com\/.*/i,
						handler: 'CacheFirst',
						options: {
							cacheName: 'game-images-cache',
							expiration: {
								maxEntries: 200,
								maxAgeSeconds: 60 * 60 * 24 * 30 // 30 days
							}
						}
					},
					{
						urlPattern: /\/api\/.*/i,
						handler: 'NetworkFirst',
						options: {
							cacheName: 'api-cache',
							expiration: {
								maxEntries: 50,
								maxAgeSeconds: 60 * 5 // 5 minutes
							},
							networkTimeoutSeconds: 3
						}
					}
				]
			},
			manifest: {
				name: 'Nexorious Game Collection',
				short_name: 'Nexorious',
				description: 'Self-hostable game collection management service for organizing and tracking your personal video game library',
				theme_color: '#3b82f6',
				background_color: '#ffffff',
				display: 'standalone',
				orientation: 'portrait-primary',
				scope: '/',
				start_url: '/',
				categories: ['games', 'entertainment', 'productivity'],
				lang: 'en',
				dir: 'ltr',
				icons: [
					{
						src: '/favicon.ico',
						sizes: '32x32',
						type: 'image/x-icon'
					},
					{
						src: '/icon-192x192.png',
						sizes: '192x192',
						type: 'image/png',
						purpose: 'any maskable'
					},
					{
						src: '/icon-512x512.png',
						sizes: '512x512',
						type: 'image/png',
						purpose: 'any maskable'
					}
				],
				shortcuts: [
					{
						name: 'Add Game',
						description: 'Add a new game to your collection',
						url: '/games/add',
						icons: [
							{
								src: '/icon-192x192.png',
								sizes: '192x192',
								type: 'image/png'
							}
						]
					},
					{
						name: 'Library',
						description: 'View your game library',
						url: '/games',
						icons: [
							{
								src: '/icon-192x192.png',
								sizes: '192x192',
								type: 'image/png'
							}
						]
					}
				]
			},
			devOptions: {
				enabled: true,
				type: 'module'
			}
		})
	]
});
