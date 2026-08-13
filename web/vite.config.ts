import { paraglideVitePlugin } from '@inlang/paraglide-js';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import basicSsl from '@vitejs/plugin-basic-ssl';
import { defineConfig } from 'vite';

// Dev-only: where `vite dev` proxies control's endpoints. The browser only
// ever talks to this (trusted, basic-ssl) dev origin, so control's self-signed
// cert never needs manual acceptance. Override with VITE_DEV_CONTROL_URL.
const DEV_CONTROL_TARGET = process.env.VITE_DEV_CONTROL_URL || 'https://127.0.0.1:8081';
const DEV_AUTH_TOKEN = process.env.PM_DEV_AUTH_TOKEN || '';

export default defineConfig({
	define: {
		__APP_VERSION__: JSON.stringify(process.env.APP_VERSION || 'dev'),
		__MARKETPLACE_URL__: JSON.stringify(
			process.env.PUBLIC_MARKETPLACE_URL || 'https://marketplace.power-manage.manchtools.com'
		),
		__BASE_PATH__: JSON.stringify(process.env.BASE_PATH || '/')
	},
	resolve: {
		// The SDK's generated TypeScript files live outside web/ (in ../sdk/gen/ts/)
		// and import @bufbuild/protobuf. Dedupe ensures Vite resolves these from
		// web/node_modules instead of looking relative to the SDK directory.
		dedupe: ['@bufbuild/protobuf']
	},
	server: {
		// This proxy holds PM_DEV_AUTH_TOKEN and can mint administrator
		// sessions, so it must never accept a non-loopback client.
		host: '127.0.0.1',
		// Vite 5.4+ rejects unknown Host headers to protect against DNS
		// rebinding. The marketplace iframe flow depends on accessing
		// this dev server through `pm.localhost` (so the publisher
		// session cookie with Domain=.localhost is shared with
		// marketplace.localhost:5180). Allow *.localhost here;
		// production builds don't use Vite's dev server.
		allowedHosts: ['.localhost'],
		// Proxy control's endpoints so the browser reaches them same-origin
		// over this dev server's already-trusted cert. `secure: false` accepts
		// control's self-signed cert on the server side; the browser never sees
		// it. Dev-only — production builds don't use Vite's dev server.
		proxy: {
			'/powermanage.v1.ControlService': {
				target: DEV_CONTROL_TARGET,
				changeOrigin: true,
				secure: false
			},
			'/dev/session': {
				target: DEV_CONTROL_TARGET,
				changeOrigin: true,
				secure: false,
				xfwd: true,
				headers: DEV_AUTH_TOKEN ? { 'X-Power-Manage-Dev-Auth': DEV_AUTH_TOKEN } : {}
			},
			'/health': { target: DEV_CONTROL_TARGET, changeOrigin: true, secure: false }
		}
	},
	plugins: [
		// Enable HTTPS in development for cross-origin cookie support.
		// This allows SameSite=None; Secure cookies to work when connecting
		// to a remote HTTPS server (e.g., production API during local dev).
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
				name: 'Power Manage',
				short_name: 'PowerMgmt',
				description: 'Device Power Management System',
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
								maxAgeSeconds: 60 * 60 * 24 * 30 // 30 days
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
