

import { test as base, expect, type Page, type Route, type Locator } from '@playwright/test';
import { primeStorage, primeStorageAs, type Theme } from '../showcase/bootstrap';
import { mockControlService, mockControlServiceExtras } from '../showcase/mocks';
import { REFERENCE_NOW_MS } from '../showcase/dummy';

export { expect };
export type { Theme };
export const THEMES: Theme[] = ['light', 'dark'];

export async function preparePage(page: Page, theme: Theme): Promise<void> {
	await page.clock.setFixedTime(REFERENCE_NOW_MS);
	await primeStorage(page, theme);
	await mockControlService(page);
	await mockControlServiceExtras(page);
}

export async function preparePageAs(page: Page, theme: Theme, permissions: string[]): Promise<void> {
	await page.clock.setFixedTime(REFERENCE_NOW_MS);
	await primeStorageAs(page, theme, { roleId: '01J6XYZSHOWCASEROLE0002', roleName: 'Limited', permissions });
	await mockControlService(page);
	await mockControlServiceExtras(page);
}

export async function gotoAndSettle(page: Page, path: string, waitFor?: string): Promise<void> {
	await page.goto(path, { waitUntil: 'networkidle' });
	if (waitFor) {

		await page.locator(`${waitFor} >> visible=true`).first().waitFor({ state: 'visible', timeout: 15_000 });
	}

	await page.waitForTimeout(250);
}

export async function failRpc(page: Page, method: string, message = 'mock backend failure'): Promise<void> {
	await page.route(`**/cadestro.v1.ControlService/${method}`, async (route: Route) => {
		await route.fulfill({
			status: 500,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ code: 'internal', message }),
		});
	});
}

export async function expectErrorToast(page: Page): Promise<void> {
	await expect(page.locator('[data-sonner-toast][data-type="error"]').first()).toBeVisible({ timeout: 10_000 });
}

export async function expectSuccessToast(page: Page): Promise<void> {
	await expect(page.locator('[data-sonner-toast][data-type="success"]').first()).toBeVisible({ timeout: 10_000 });
}

export function recordRpc<T = Record<string, unknown>>(page: Page, method: string): T[] {
	const calls: T[] = [];
	void page.route(`**/cadestro.v1.ControlService/${method}`, async (route) => {
		try {
			calls.push(route.request().postDataJSON() as T);
		} catch {
			calls.push({} as T);
		}
		await route.fallback();
	});
	return calls;
}

export async function clickUntil(target: Locator, appears: Locator, tries = 6): Promise<void> {
	for (let i = 0; i < tries; i++) {
		await target.click({ timeout: 5000 }).catch(() => {});
		try {
			await appears.waitFor({ state: 'visible', timeout: 1500 });
			return;
		} catch {

		}
	}
	await appears.waitFor({ state: 'visible', timeout: 5000 });
}

export const test = base;
