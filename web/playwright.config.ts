import { defineConfig, devices } from '@playwright/test';

// Playwright config for two harnesses under tests/, both driving the SvelteKit
// dev server with the Control API fully mocked (Connect-RPC interception):
//
//   tests/e2e/      — behavioural / interaction tests (clicks → asserted via the
//                     RPC they emit + DOM state). Run: `npm run test:e2e`.
//   tests/showcase/ — documentation / marketing screenshot generation
//                     (raw page.screenshot). Run: `npx playwright test tests/showcase`.
//
// Local-only — not wired into CI.
export default defineConfig({
	testDir: 'tests',
	timeout: 60_000,
	expect: { timeout: 10_000 },
	fullyParallel: false,
	workers: 1,
	reporter: [['list']],
	outputDir: 'tests/.test-output',
	use: {
		// vite.config.ts has basicSsl() enabled; ignoreHTTPSErrors lets us
		// accept the self-signed cert. 5179 avoids collisions with other dev
		// servers on 5173/5180.
		baseURL: 'https://localhost:5179',
		ignoreHTTPSErrors: true,
		// en-GB gives a 24h clock + DD/MM/YYYY ordering with English month
		// names. Europe/Berlin pins the timezone so reruns are identical.
		locale: 'en-GB',
		timezoneId: 'Europe/Berlin',
		trace: 'off',
		video: 'off',
	},
	webServer: {
		command: 'npm run dev -- --port 5179 --strictPort --host 127.0.0.1',
		url: 'https://localhost:5179',
		ignoreHTTPSErrors: true,
		reuseExistingServer: true,
		timeout: 180_000,
	},
	projects: [
		{
			name: 'chromium',
			use: {
				...devices['Desktop Chrome'],
				// Pin viewport + DPR explicitly — the device preset would
				// otherwise set its own (1280×720), making snapshots depend on
				// which preset ships with the installed Playwright version.
				viewport: { width: 1440, height: 900 },
				deviceScaleFactor: 1,
			},
		},
	],
});
