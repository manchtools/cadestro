// Vitest config. Two projects:
//
//   unit    — pure-logic + store tests in node (fast). Files: *.test.ts /
//             *.spec.ts / *.svelte.test.ts, EXCLUDING *.browser.test.ts.
//   browser — real-component renders in headless Chromium via the Playwright
//             provider (spec 001). Files: *.browser.test.ts. Real CSS
//             (Tailwind) is loaded so layout/pointer/visibility are faithful —
//             jsdom was rejected for faking exactly that.
//
// Both inherit the svelte-kit + paraglide + tailwind vite plugins and the app's
// `define` globals (`extends: true`) so `$lib` aliases, compiled messages,
// utility classes, and `__APP_VERSION__` resolve as the production build does.

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
	// Keep a single Svelte instance in browser tests. Pre-bundling `@lucide/svelte`
	// links its icons against a second copy of Svelte's client runtime, so an icon
	// mount throws `get_first_child … reading 'call'` and aborts the whole render.
	// Excluding it (and deduping svelte) makes icons compile through the same
	// pipeline as the app.
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
					// Browser files install many module-mock routes. Keep one Playwright
					// context active per shard so their route lifecycles cannot overlap.
					maxWorkers: 1,
					browser: {
						enabled: true,
						provider: playwright(),
						headless: true,
						// Desktop viewport — the shell is desktop chrome. The default is
						// narrow enough that a fixed w-96 window's right-aligned header
						// controls land off-screen and become unclickable.
						viewport: { width: 1280, height: 800 },
						instances: [{ browser: 'chromium' }]
					}
				}
			}
		]
	}
});
