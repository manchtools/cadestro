

import { test, expect, preparePageAs, gotoAndSettle, clickUntil } from './fixtures';

test('nav exposes only the sections the session is permitted', async ({ page }) => {

	await preparePageAs(page, 'light', ['ListDevices', 'ListActions', 'Search']);
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	await expect(page.locator('a[href="/devices"]')).not.toHaveCount(0);
	await expect(page.locator('a[href="/actions"]')).not.toHaveCount(0);

	await clickUntil(page.getByRole('button', { name: 'More' }), page.locator('a[href="/my-devices"]'));
	await expect(page.locator('a[href="/users"]')).toHaveCount(0);
	await expect(page.locator('a[href="/roles"]')).toHaveCount(0);
});

test('granting ListUsers reveals the Users section, still not Roles', async ({ page }) => {
	await preparePageAs(page, 'light', ['ListDevices', 'ListUsers', 'Search']);
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	await clickUntil(page.getByRole('button', { name: 'More' }), page.locator('a[href="/users"]'));
	await expect(page.locator('a[href="/users"]')).not.toHaveCount(0);
	await expect(page.locator('a[href="/roles"]')).toHaveCount(0);
});
