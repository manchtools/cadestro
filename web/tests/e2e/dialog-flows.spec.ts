

import { test, expect, preparePage, gotoAndSettle, recordRpc, expectSuccessToast, clickUntil } from './fixtures';

test('deleting a device confirms then calls DeleteDevice with that id', async ({ page }) => {
	await preparePage(page, 'light');
	const del = recordRpc<{ id?: string }>(page, 'DeleteDevice');
	await gotoAndSettle(page, '/devices', 'table tbody tr');

	const menuTrigger = page.locator('table tbody tr').first().locator('button').last();
	const deleteItem = page.getByRole('menuitem', { name: 'Delete' });
	await clickUntil(menuTrigger, deleteItem);
	await deleteItem.click();

	const confirm = page.getByRole('alertdialog').getByRole('button', { name: 'Delete' });
	await confirm.waitFor({ state: 'visible', timeout: 5000 });
	await confirm.click();

	await expect.poll(() => del.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(del[0].id).toBe('01J6XYZSHOWCASEDEVICE0001');
	await expectSuccessToast(page);
});

test('creating a role submits CreateRole with the entered name', async ({ page }) => {
	await preparePage(page, 'light');
	const create = recordRpc<{ name?: string }>(page, 'CreateRole');
	await gotoAndSettle(page, '/roles', 'text=Administrator');

	const nameField = page.getByRole('dialog').getByLabel(/name/i).first();
	await clickUntil(page.getByRole('button', { name: /create role|new role|add role/i }).first(), nameField);

	await nameField.fill('Release Manager');
	await page.getByRole('dialog').getByRole('button', { name: /create|save|add/i }).last().click();

	await expect.poll(() => create.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(create[0].name).toBe('Release Manager');
});
