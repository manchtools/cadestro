// Shared setup for the visual-regression + error-handling e2e suite.
//
// Every test runs against the SvelteKit dev server with the Control API fully
// mocked (Connect-RPC route interception — see ../showcase/mocks.ts) and an
// admin session pre-seeded into localStorage (../showcase/bootstrap.ts), so
// tests never touch a real backend and skip the login flow.
//
// Determinism is everything for pixel snapshots: the browser clock is frozen
// to the same instant the dummy data is built around (REFERENCE_NOW_MS), so
// relative timestamps ("5 minutes ago") render identically on every run.
// setFixedTime keeps timers running, so the list pages' 300ms search debounce
// still fires.

import { test as base, expect, type Page, type Route, type Locator } from '@playwright/test';
import { primeStorage, primeStorageAs, type Theme } from '../showcase/bootstrap';
import { mockControlService, mockControlServiceExtras } from '../showcase/mocks';
import { REFERENCE_NOW_MS } from '../showcase/dummy';

export { expect };
export type { Theme };
export const THEMES: Theme[] = ['light', 'dark'];

// Prepare a page for a deterministic render: freeze the clock, prime the admin
// session + theme, and install the RPC mocks. Call before navigating.
export async function preparePage(page: Page, theme: Theme): Promise<void> {
	await page.clock.setFixedTime(REFERENCE_NOW_MS);
	await primeStorage(page, theme);
	await mockControlService(page);
	await mockControlServiceExtras(page);
}

// Same as preparePage but with a non-admin session carrying exactly the given
// permissions — for RBAC / permission-gating assertions. Use on pages that read
// the seeded session directly (e.g. the nav layout); pages that call
// GetCurrentUser would be re-broadened by that mock.
export async function preparePageAs(page: Page, theme: Theme, permissions: string[]): Promise<void> {
	await page.clock.setFixedTime(REFERENCE_NOW_MS);
	await primeStorageAs(page, theme, { roleId: '01J6XYZSHOWCASEROLE0002', roleName: 'Limited', permissions });
	await mockControlService(page);
	await mockControlServiceExtras(page);
}

// Navigate and settle: wait for the network to go idle and for the route's
// landmark element to be visible, so the snapshot is taken against a fully
// rendered page rather than a loading skeleton.
export async function gotoAndSettle(page: Page, path: string, waitFor?: string): Promise<void> {
	await page.goto(path, { waitUntil: 'networkidle' });
	if (waitFor) {
		// `>> visible=true` selects the first *visible* match. Some landmarks
		// (e.g. `main`) appear twice — a hidden responsive variant plus the
		// real one — and a plain `.first()` can latch onto the hidden copy.
		await page.locator(`${waitFor} >> visible=true`).first().waitFor({ state: 'visible', timeout: 15_000 });
	}
	// Settle late layout shifts (font swap, async icons) before the snapshot.
	await page.waitForTimeout(250);
}

// Force one ControlService RPC to return a Connect error, so a mutation
// triggered in the UI exercises the page's error-handling path. Register AFTER
// preparePage so it wins over the success mock (page.route is LIFO). A Connect
// unary error is an HTTP 5xx with a JSON `{ code, message }` body, which
// connect-es surfaces to the app as a thrown ConnectError.
export async function failRpc(page: Page, method: string, message = 'mock backend failure'): Promise<void> {
	await page.route(`**/powermanage.v1.ControlService/${method}`, async (route: Route) => {
		await route.fulfill({
			status: 500,
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ code: 'internal', message }),
		});
	});
}

// Assert a destructive/mutation failure surfaced an error toast to the user
// (svelte-sonner with richColors tags error toasts data-type="error").
export async function expectErrorToast(page: Page): Promise<void> {
	await expect(page.locator('[data-sonner-toast][data-type="error"]').first()).toBeVisible({ timeout: 10_000 });
}

export async function expectSuccessToast(page: Page): Promise<void> {
	await expect(page.locator('[data-sonner-toast][data-type="success"]').first()).toBeVisible({ timeout: 10_000 });
}

// Records the decoded request body of every call to one ControlService RPC and
// then defers to the already-registered mock (route.fallback() continues down
// the LIFO chain), so capturing a request never changes the response. The
// returned array grows as calls arrive — assert on it with expect.poll.
//
// This is how interaction tests stay deterministic: a click is verified by the
// exact request it produced (e.g. sortField / tagFilters), not by pixels.
export function recordRpc<T = Record<string, unknown>>(page: Page, method: string): T[] {
	const calls: T[] = [];
	void page.route(`**/powermanage.v1.ControlService/${method}`, async (route) => {
		try {
			calls.push(route.request().postDataJSON() as T);
		} catch {
			calls.push({} as T);
		}
		await route.fallback();
	});
	return calls;
}

// Click `target` until `appears` is visible. bits-ui menus/dialogs occasionally
// drop the first click under Svelte 5 + Playwright, so opening clicks retry.
export async function clickUntil(target: Locator, appears: Locator, tries = 6): Promise<void> {
	for (let i = 0; i < tries; i++) {
		await target.click({ timeout: 5000 }).catch(() => {});
		try {
			await appears.waitFor({ state: 'visible', timeout: 1500 });
			return;
		} catch {
			/* retry */
		}
	}
	await appears.waitFor({ state: 'visible', timeout: 5000 });
}

export const test = base;
