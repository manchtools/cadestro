import { paraglideVitePlugin } from '@inlang/paraglide-js';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import basicSsl from '@vitejs/plugin-basic-ssl';
import { defineConfig } from 'vite';

const DEV_CONTROL_TARGET = process.env.VITE_DEV_CONTROL_URL || 'https://127.0.0.1:8081';

export default defineConfig({
	define: {
		__APP_VERSION__: JSON.stringify(process.env.APP_VERSION || 'dev'),
		__MARKETPLACE_URL__: JSON.stringify(
			process.env.PUBLIC_MARKETPLACE_URL || 'https://marketplace.cadestro.manchtools.com'
		),
		__BASE_PATH__: JSON.stringify(process.env.BASE_PATH || '/')
	},
	resolve: {

		dedupe: ['@bufbuild/protobuf']
	},
	server: {
		host: '127.0.0.1',

		allowedHosts: ['.localhost'],

		proxy: {
			'/cadestro.v1.ControlService': {
				target: DEV_CONTROL_TARGET,
				changeOrigin: true,
				secure: false
			},
			'/health': { target: DEV_CONTROL_TARGET, changeOrigin: true, secure: false }
		}
	},
	plugins: [

		basicSsl(),
		paraglideVitePlugin({
			project: './project.inlang',
			outdir: './src/lib/paraglide'
		}),
		tailwindcss(),
		sveltekit(),
		SvelteKitPWA({
			srcDir: 'src',
			mode: 'production',
			strategies: 'generateSW',
			scope: process.env.BASE_PATH || '/',
			base: process.env.BASE_PATH || '/',
			registerType: 'autoUpdate',
			manifest: {
				name: 'Cadestro',
				short_name: 'Cadestro',
				description: 'Linux fleet management',
				theme_color: '#171717',
				background_color: '#171717',
				display: 'standalone',
				start_url: process.env.BASE_PATH ? process.env.BASE_PATH + '/' : '/',
				scope: process.env.BASE_PATH || '/',
				icons: [
					{
						src: '/icon.svg',
						sizes: 'any',
						type: 'image/svg+xml',
						purpose: 'any maskable'
					}
				]
			},
			workbox: {
				globPatterns: ['**/*.{js,css,html,ico,png,svg,woff,woff2}'],
				runtimeCaching: [
					{
						urlPattern: /^https:\/\/.*\.(?:png|jpg|jpeg|svg|gif)$/,
						handler: 'CacheFirst',
						options: {
							cacheName: 'images',
							expiration: {
								maxEntries: 100,
								maxAgeSeconds: 60 * 60 * 24 * 30
							}
						}
					}
				]
			},
			devOptions: {
				enabled: false,
				type: 'module'
			}
		})
	]
});
