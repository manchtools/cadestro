import { test } from '@playwright/test';
import { primeStorage, type Theme } from './bootstrap';
import { mockControlService } from './mocks';

const THEMES: Theme[] = ['light', 'dark'];

for (const theme of THEMES) {
	test(`actions list — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/actions', { waitUntil: 'networkidle' });
		await page.waitForSelector('table tbody tr', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/actions-${theme}.png`,
			fullPage: false,
		});
	});
}
