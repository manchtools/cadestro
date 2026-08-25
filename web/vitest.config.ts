

import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { paraglideVitePlugin } from '@inlang/paraglide-js';
import { playwright } from '@vitest/browser-playwright';

const pwaStub = fileURLToPath(new URL('./src/lib/test/pwa-stub.ts', import.meta.url));

export default defineConfig({
	define: {
		__APP_VERSION__: JSON.stringify(process.env.APP_VERSION || 'test'),
		__MARKETPLACE_URL__: JSON.stringify('https://marketplace.example.test'),
		__BASE_PATH__: JSON.stringify('/')
	},

	resolve: {
		alias: {
			'virtual:pwa-info': pwaStub,
			'virtual:pwa-register': pwaStub
		},
		dedupe: ['svelte']
	},
	optimizeDeps: { exclude: ['@lucide/svelte'] },
	plugins: [
		paraglideVitePlugin({
			project: './project.inlang',
			outdir: './src/lib/paraglide'
		}),
		tailwindcss(),
		sveltekit()
	],
	test: {
		projects: [
			{
				extends: true,
				test: {
					name: 'unit',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{ts,svelte.ts}'],
					exclude: ['src/**/*.browser.test.ts'],
					globals: false
				}
			},
			{
				extends: true,
				test: {
					name: 'browser',
					include: ['src/**/*.browser.test.ts'],
					setupFiles: ['./src/lib/test/browser-setup.ts'],

					maxWorkers: 1,
					browser: {
						enabled: true,
						provider: playwright(),
						headless: true,

						viewport: { width: 1280, height: 800 },
						instances: [{ browser: 'chromium' }]
					}
				}
			}
		]
	}
});
