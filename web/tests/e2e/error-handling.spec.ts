// Mutation error-handling suite. For each case we force one mutation RPC to
// return a Connect error, fire that mutation through the UI, and assert the
// page surfaces an error toast rather than failing silently. This guards the
// error path that visual snapshots can't reach.
//
// Adding a case: append to CASES below — a page path, the RPC to fail, and the
// click that triggers it. Single-click mutations (a button) are the most
// robust; dialog-driven mutations (create/delete) can be added with the extra
// open-dialog → confirm steps in `trigger`.

import { test, preparePage, gotoAndSettle, failRpc, expectErrorToast, clickUntil } from './fixtures';
import type { Page } from '@playwright/test';

type MutationErrorCase = {
	name: string;
	path: string;
	waitFor?: string;
	failRpc: string; // ControlService method forced to return an error
	trigger: (page: Page) => Promise<void>; // UI steps that fire the mutation
};

const CASES: MutationErrorCase[] = [
	{
		name: 'device inventory refresh',
		path: '/devices/01J6XYZSHOWCASEDEVICE0001',
		waitFor: 'text=edge-01.berlin',
		failRpc: 'RefreshDeviceInventory',
		trigger: (page) => page.getByRole('button', { name: 'Refresh Inventory' }).click(),
	},
	{
		name: 'rebuild search index',
		path: '/settings',
		waitFor: 'text=Rebuild Search Index',
		failRpc: 'RebuildSearchIndex',
		trigger: (page) => page.getByRole('button', { name: 'Rebuild Search Index' }).click(),
	},
	{
		// A dialog-driven destructive mutation: open the row menu → Delete →
		// confirm → DeleteDevice fails → the page must surface the error.
		name: 'delete device (confirm dialog)',
		path: '/devices',
		waitFor: 'table tbody tr',
		failRpc: 'DeleteDevice',
		trigger: async (page) => {
			const deleteItem = page.getByRole('menuitem', { name: 'Delete' });
			await clickUntil(page.locator('table tbody tr').first().locator('button').last(), deleteItem);
			await deleteItem.click();
			await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click();
		},
	},
];

for (const c of CASES) {
	test(`mutation error surfaced — ${c.name}`, async ({ page }) => {
		await preparePage(page, 'light');
		await failRpc(page, c.failRpc);
		await gotoAndSettle(page, c.path, c.waitFor);
		await c.trigger(page);
		await expectErrorToast(page);
	});
}
