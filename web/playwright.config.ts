import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: 'tests',
	timeout: 60_000,
	expect: { timeout: 10_000 },
	fullyParallel: false,
	workers: 1,
	reporter: [['list']],
	outputDir: 'tests/.test-output',
	use: {

		baseURL: 'https://localhost:5179',
		ignoreHTTPSErrors: true,

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

				viewport: { width: 1440, height: 900 },
				deviceScaleFactor: 1,
			},
		},
	],
});
