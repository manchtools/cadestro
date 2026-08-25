

import { test, expect, preparePage, preparePageAs, gotoAndSettle, clickUntil } from './fixtures';

test('canonical routes render the shell chrome, not the sidebar', async ({ page }) => {
	await preparePage(page, 'light');
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	await expect(page.getByTestId('morph-bar')).toBeVisible();
	await expect(page.getByTestId('morph-bar')).toHaveAttribute('data-mode', 'nav');

	await expect(page.locator('[data-sidebar="sidebar"]')).toHaveCount(0);
});

test('/next does not exist', async ({ page }) => {
	await preparePage(page, 'light');
	const resp = await page.goto('/next/devices');
	expect(resp?.status()).toBe(404);
});

test('More ▾ overflow lists the other permitted sections and navigates', async ({ page }) => {
	await preparePage(page, 'light');
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	await clickUntil(page.getByRole('button', { name: 'More' }), page.locator('a[href="/users"]'));
	await expect(page.locator('a[href="/user-groups"]')).toBeVisible();
	await page.locator('a[href="/users"]').click();
	await expect(page).toHaveURL(/\/users$/);
	await expect(page.getByTestId('morph-bar')).toBeVisible();
});

test('permission filtering: pill and overflow expose only granted sections', async ({ page }) => {
	await preparePageAs(page, 'light', ['ListDevices', 'ListActions', 'Search']);
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	const bar = page.getByTestId('morph-bar');
	await expect(bar.locator('a[href="/devices"]')).toBeVisible();
	await expect(bar.locator('a[href="/actions"]')).toBeVisible();

	await expect(bar.locator('a[href="/audit"]')).toHaveCount(0);

	await clickUntil(page.getByRole('button', { name: 'More' }), page.locator('a[href="/my-devices"]'));
	await expect(page.locator('a[href="/users"]')).toHaveCount(0);
	await expect(page.locator('a[href="/roles"]')).toHaveCount(0);
});

test('shell surfaces survive navigation without remounting', async ({ page }) => {
	await preparePage(page, 'light');
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	await page.evaluate(() => {
		document.querySelector('[data-testid="morph-bar"]')?.setAttribute('data-identity', 'survivor');
	});
	await page.getByTestId('morph-bar').locator('a[href="/actions"]').click();
	await expect(page).toHaveURL(/\/actions$/);

	await expect(page.locator('[data-testid="morph-bar"][data-identity="survivor"]')).toBeVisible();
});

test('a device window remains alive across navigation and can be staged', async ({ page }) => {
	await preparePage(page, 'light');
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	const firstRow = page.locator('table tbody tr').first();
	await clickUntil(firstRow.getByRole('button', { name: 'Actions' }), page.getByText('Open in window'));
	await page.getByText('Open in window').click();

	const panel = page.getByTestId('panel');
	await expect(panel).toBeVisible();
	await page.evaluate(() => {
		document.querySelector('[data-testid="panel"]')?.setAttribute('data-identity', 'device-window');
	});

	await page.getByTestId('morph-bar').locator('a[href="/actions"]').click();
	await expect(page).toHaveURL(/\/actions$/);
	await expect(page.locator('[data-testid="panel"][data-identity="device-window"]')).toBeVisible();

	await panel.getByRole('button', { name: 'Minimise' }).click();
	await expect(page.getByTestId('stage-rail')).toBeVisible();
	await page.getByTestId('stage-card').click();
	await expect(page.locator('[data-testid="panel"][data-identity="device-window"]')).toBeVisible();
});
