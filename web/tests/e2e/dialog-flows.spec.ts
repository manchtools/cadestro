

import { test, expect, preparePage, gotoAndSettle, recordRpc, expectSuccessToast } from './fixtures';

test('deleting an action set confirms then calls DeleteActionSet with that id', async ({ page }) => {
	await preparePage(page, 'light');
	const del = recordRpc<{ id?: { value?: string } }>(page, 'DeleteActionSet');
	await gotoAndSettle(page, '/action-sets?zoom=list', '[data-testid="row-list-row"]');

	const menuTrigger = page.getByTestId('row-list-row').first().locator('button').last();
	const deleteItem = page.getByRole('menuitem', { name: 'Delete' });
	await menuTrigger.click();
	await deleteItem.click();

	const confirm = page.getByRole('alertdialog').getByRole('button', { name: 'Delete' });
	await confirm.waitFor({ state: 'visible', timeout: 5000 });
	await confirm.click();

	await expect.poll(() => del.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(del[0].id?.value).toBe('01J6XYZSHOWCASESET00001');
	await expectSuccessToast(page);
});

test('creating a role submits CreateRole with the entered name', async ({ page }) => {
	await preparePage(page, 'light');
	const create = recordRpc<{ name?: string }>(page, 'CreateRole');
	await gotoAndSettle(page, '/roles/new', 'input#role-name');

	const nameField = page.locator('#role-name');

	await nameField.fill('Release Manager');
	await page.getByTestId('pill-commit').click();

	await expect.poll(() => create.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(create[0].name).toBe('Release Manager');
});
