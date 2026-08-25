

import { test, preparePage, gotoAndSettle, failRpc, expectErrorToast, clickUntil } from './fixtures';
import type { Page } from '@playwright/test';

type MutationErrorCase = {
	name: string;
	path: string;
	waitFor?: string;
	failRpc: string;
	trigger: (page: Page) => Promise<void>;
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
		name: 'delete action set (confirm dialog)',
		path: '/action-sets?zoom=list',
		waitFor: '[data-testid="row-list-row"]',
		failRpc: 'DeleteActionSet',
		trigger: async (page) => {
			const deleteItem = page.getByRole('menuitem', { name: 'Delete' });
			await clickUntil(page.getByTestId('row-list-row').first().locator('button').last(), deleteItem);
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
