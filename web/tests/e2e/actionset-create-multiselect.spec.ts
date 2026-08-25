

import { test, expect, preparePage, gotoAndSettle, recordRpc, clickUntil } from './fixtures';
import type { Page, Route } from '@playwright/test';

async function mockCreateFlow(page: Page): Promise<void> {
	await page.route('**/cadestro.v1.ControlService/CreateActionSet', async (route: Route) => {
		await route.fulfill({
			status: 200,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ set: { id: '01JREPROSET000000000000000', name: 'repro', memberCount: 0 } }),
		});
	});
	await page.route('**/cadestro.v1.ControlService/AddActionToSet', async (route: Route) => {
		await route.fulfill({
			status: 200,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ set: { id: '01JREPROSET000000000000000', name: 'repro', memberCount: 1 } }),
		});
	});
}

async function openDialogToActionsStep(page: Page): Promise<void> {
	await gotoAndSettle(page, '/action-sets', 'main');
	const createBtn = page.getByRole('button', { name: 'Create Action Set' }).first();
	await clickUntil(createBtn, page.getByRole('dialog'));
	await page.getByRole('dialog').getByLabel(/name/i).first().fill('repro set');

	await page.getByRole('dialog').getByRole('button', { name: /next|continue|actions/i }).first().click();
	await page.getByRole('dialog').locator('table tbody tr').first().waitFor({ state: 'visible', timeout: 10_000 });
}

test('create dialog: selecting two actions via ROW clicks adds two', async ({ page }) => {
	await preparePage(page, 'light');
	await mockCreateFlow(page);
	const adds = recordRpc(page, 'AddActionToSet');

	await openDialogToActionsStep(page);
	const rows = page.getByRole('dialog').locator('table tbody tr');

	await rows.nth(0).locator('td').nth(1).click();
	await rows.nth(1).locator('td').nth(1).click();
	await expect(page.getByRole('dialog').getByText(/2 actions selected/)).toBeVisible();

	await page.getByRole('dialog').getByRole('button', { name: /create/i }).last().click();
	await expect.poll(() => adds.length, { timeout: 10_000 }).toBe(2);
});

test('create dialog: selecting two actions via CHECKBOX clicks adds two', async ({ page }) => {
	await preparePage(page, 'light');
	await mockCreateFlow(page);
	const adds = recordRpc(page, 'AddActionToSet');

	await openDialogToActionsStep(page);
	const rows = page.getByRole('dialog').locator('table tbody tr');
	await rows.nth(0).getByRole('checkbox').click();
	await rows.nth(1).getByRole('checkbox').click();
	await expect(page.getByRole('dialog').getByText(/2 actions selected/)).toBeVisible({ timeout: 5_000 });

	await page.getByRole('dialog').getByRole('button', { name: /create/i }).last().click();
	await expect.poll(() => adds.length, { timeout: 10_000 }).toBe(2);
});
