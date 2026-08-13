// A non-admin session is seeded with an explicit permission set, and the tests
// assert the pill + More overflow gate
// accordingly — a section's link is present only when the session holds its
// List* permission. Denied-link assertions hold with the overflow OPEN, so a
// closed menu can never false-pass them. (Admins short-circuit the check, so
// these sessions use a non-admin role.)

import { test, expect, preparePageAs, gotoAndSettle, clickUntil } from './fixtures';

test('nav exposes only the sections the session is permitted', async ({ page }) => {
	// Granted: devices + actions. Denied: users + roles.
	await preparePageAs(page, 'light', ['ListDevices', 'ListActions', 'Search']);
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	await expect(page.locator('a[href="/devices"]')).not.toHaveCount(0);
	await expect(page.locator('a[href="/actions"]')).not.toHaveCount(0);
	// open the overflow: denied links must be absent even with it open
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
