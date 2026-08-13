import { test } from '@playwright/test';
import { primeStorage, type Theme } from './bootstrap';
import { mockControlService } from './mocks';

const THEMES: Theme[] = ['light', 'dark'];

for (const theme of THEMES) {
	test(`device groups list — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/device-groups', { waitUntil: 'networkidle' });
		await page.waitForSelector('table tbody tr', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/device-groups-${theme}.png`,
			fullPage: false,
		});
	});

	test(`device group detail (dynamic) — ${theme}`, async ({ page }) => {
		await primeStorage(page, theme);
		await mockControlService(page);
		await page.goto('/device-groups/01J6XYZSHOWCASEDEVGRP0001', { waitUntil: 'networkidle' });
		// The dynamic-query Card heading appears once GetDeviceGroup resolves
		// and the page knows isDynamic=true.
		await page.waitForSelector('text=Berlin Edge Nodes', { timeout: 15_000 });
		await page.screenshot({
			path: `tests/showcase/output/device-group-dynamic-${theme}.png`,
			fullPage: false,
		});
	});
}
