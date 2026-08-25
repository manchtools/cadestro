import { test } from '@playwright/test';
import { primeStorage, type Theme } from './bootstrap';
import { mockControlService } from './mocks';

const THEMES: Theme[] = ['light', 'dark'];

for (const theme of THEMES) {
	test(`users list — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/users', { waitUntil: 'networkidle' });
		await page.waitForSelector('table tbody tr', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/users-${theme}.png`,
			fullPage: false,
		});
	});

	test(`roles list — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/roles', { waitUntil: 'networkidle' });

		await page.waitForSelector('text=Fleet Operator', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/roles-${theme}.png`,
			fullPage: false,
		});
	});

	test(`user-groups list — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/user-groups', { waitUntil: 'networkidle' });
		await page.waitForSelector('table tbody tr', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/user-groups-${theme}.png`,
			fullPage: false,
		});
	});
}
