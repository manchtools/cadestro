import { test } from '@playwright/test';
import { primeStorage, type Theme } from './bootstrap';
import { mockControlService } from './mocks';

const THEMES: Theme[] = ['light', 'dark'];

for (const theme of THEMES) {
	test(`devices list — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/devices', { waitUntil: 'networkidle' });
		await page.waitForSelector('table tbody tr', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/devices-${theme}.png`,
			fullPage: false,
		});
	});

	test(`device detail — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/devices/01J6XYZSHOWCASEDEVICE0001', { waitUntil: 'networkidle' });
		await page.waitForSelector('text=edge-01.berlin', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/device-detail-${theme}.png`,
			fullPage: false,
		});
	});

	test(`device detail — compliance tab — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/devices/01J6XYZSHOWCASEDEVICE0001', { waitUntil: 'networkidle' });
		await page.waitForSelector('text=edge-01.berlin', { timeout: 15_000 });

		await page.getByRole('tab', { name: 'Compliance' }).click();
		await page.waitForSelector('text=CIS Linux Baseline', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/device-compliance-${theme}.png`,
			fullPage: false,
		});
	});
}
