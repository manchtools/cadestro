

import { test, expect, preparePage, gotoAndSettle, recordRpc } from './fixtures';
import type { Page, Route } from '@playwright/test';
import { DUMMY_ACTIONS, REFERENCE_NOW_MS, wrappedID } from '../showcase/dummy';

async function mockCreateFlow(page: Page): Promise<void> {
	await page.route('**/cadestro.v1.ControlService/ListActions', async (route: Route) => {
		const actions = DUMMY_ACTIONS.slice(0, 2).map((action) => ({
			id: wrappedID(action.id),
			name: action.name,
			description: action.description,
			type: action.type,
			desiredState: action.desired_state,
			createdAt: new Date(REFERENCE_NOW_MS - 60 * 86400000).toISOString(),
			updatedAt: new Date(REFERENCE_NOW_MS - 12 * 86400000).toISOString()
		}));
		await route.fulfill({
			status: 200,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ actions, nextPageToken: '', totalCount: actions.length }),
		});
	});
	await page.route('**/cadestro.v1.ControlService/CreateActionSet', async (route: Route) => {
		await route.fulfill({
			status: 200,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ set: { id: wrappedID('01JREPROSET000000000000000'), name: 'repro', memberCount: 0 } }),
		});
	});
	await page.route('**/cadestro.v1.ControlService/AddActionToSet', async (route: Route) => {
		await route.fulfill({
			status: 200,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ set: { id: wrappedID('01JREPROSET000000000000000'), name: 'repro', memberCount: 1 } }),
		});
	});
}

async function openDialogToActionsStep(page: Page): Promise<void> {
	await gotoAndSettle(page, '/action-sets/new', 'main');
	await page.getByLabel(/name/i).first().fill('repro set');
	await page.getByTestId('set-action-row').first().waitFor({ state: 'visible', timeout: 10_000 });
}

test('create dialog: selecting two actions via ROW clicks adds two', async ({ page }) => {
	await preparePage(page, 'light');
	await mockCreateFlow(page);
	const adds = recordRpc(page, 'AddActionToSet');

	await openDialogToActionsStep(page);
	const rows = page.getByTestId('set-action-row');

	await rows.nth(0).click();
	await rows.nth(1).click();
	await expect(page.getByTestId('set-selected-count')).toContainText('2');

	await page.getByTestId('pill-commit').click();
	await expect.poll(() => adds.length, { timeout: 10_000 }).toBe(2);
});

test('create page: selecting two actions adds two', async ({ page }) => {
	await preparePage(page, 'light');
	await mockCreateFlow(page);
	const adds = recordRpc(page, 'AddActionToSet');

	await openDialogToActionsStep(page);
	const rows = page.getByTestId('set-action-row');
	await rows.nth(0).click();
	await rows.nth(1).click();
	await expect(page.getByTestId('set-selected-count')).toContainText('2', { timeout: 5_000 });

	await page.getByTestId('pill-commit').click();
	await expect.poll(() => adds.length, { timeout: 10_000 }).toBe(2);
});
