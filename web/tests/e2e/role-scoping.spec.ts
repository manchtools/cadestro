// The role-assignment scope picker is driven by each permission's target_kind
// (ListPermissions), not a hardcoded allowlist. Verify both the available choice
// and the request sent after assigning it.

import { test, expect, preparePage, gotoAndSettle, recordRpc, clickUntil } from './fixtures';
import type { Page, Route } from '@playwright/test';

const USER_ID = '01J6XYZSHOWCASEUSER0002';

async function json(route: Route, body: unknown): Promise<void> {
	await route.fulfill({
		status: 200,
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(body),
	});
}

// Scope-picker fixtures: three roles each with one permission of a distinct
// target kind, plus a device group and a user group to scope to. Registered
// after preparePage so they win (page.route is LIFO).
async function installScopeFixtures(page: Page): Promise<void> {
	await page.route('**/cadestro.v1.ControlService/ListPermissions', (r) =>
		json(r, {
			permissions: [
				{ key: 'DevPerm', group: 'Devices', description: 'd', targetKind: 'PERMISSION_TARGET_KIND_DEVICE' },
				{ key: 'UserPerm', group: 'Users', description: 'u', targetKind: 'PERMISSION_TARGET_KIND_USER' },
				{ key: 'OrgPerm', group: 'Org', description: 'o', targetKind: 'PERMISSION_TARGET_KIND_UNSPECIFIED' },
			],
		})
	);
	await page.route('**/cadestro.v1.ControlService/ListRoles', (r) =>
		json(r, {
			roles: [
				{ id: 'ROLE_DEVICE', name: 'Device Scoped Role', description: '', permissions: ['DevPerm'], isSystem: false },
				{ id: 'ROLE_USER', name: 'User Scoped Role', description: '', permissions: ['UserPerm'], isSystem: false },
				{ id: 'ROLE_ORG', name: 'Org Wide Role', description: '', permissions: ['OrgPerm'], isSystem: false },
			],
			nextPageToken: '',
			totalCount: 3,
		})
	);
	await page.route('**/cadestro.v1.ControlService/ListDeviceGroups', (r) =>
		json(r, { groups: [{ id: 'DG_PROD', name: 'Production', description: '', memberCount: 0 }], nextPageToken: '', totalCount: 1 })
	);
	await page.route('**/cadestro.v1.ControlService/ListUserGroups', (r) =>
		json(r, { groups: [{ id: 'UG_ADMINS', name: 'Admins', description: '', memberCount: 0 }], nextPageToken: '', totalCount: 1 })
	);
}

type AssignReq = { roleIds?: string[]; scopeKind?: string | number; scopeId?: string };

// Open the Assign Role dialog and select one role by name.
async function openDialogAndSelect(page: Page, roleName: string): Promise<void> {
	const trigger = page.getByRole('button', { name: 'Assign Role' }).first();
	const roleRow = page.getByRole('dialog').getByText(roleName);
	await clickUntil(trigger, roleRow);
	await roleRow.click();
}

const scopePicker = (page: Page) =>
	page.getByRole('dialog').getByRole('combobox', { name: 'Scope' });

async function chooseScope(page: Page, name: string): Promise<void> {
	await scopePicker(page).click();
	await page.getByRole('option', { name, exact: true }).click();
}

test('DEVICE-kind role offers a device-group picker and assigns with DEVICE_GROUP scope', async ({ page }) => {
	await preparePage(page, 'light');
	await installScopeFixtures(page);
	await page.route('**/cadestro.v1.ControlService/AssignRoleToUser', (r) => json(r, {}));
	const calls = recordRpc<AssignReq>(page, 'AssignRoleToUser');

	await gotoAndSettle(page, `/users/${USER_ID}`, 'text=Assign Role');
	await openDialogAndSelect(page, 'Device Scoped Role');

	// The scope picker appears (device kind) and offers the device group.
	await expect(scopePicker(page)).toBeEnabled();
	await chooseScope(page, 'Production');

	await page.getByRole('dialog').getByRole('button', { name: 'Assign', exact: true }).click();

	await expect.poll(() => calls.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(calls[0].roleIds).toEqual(['ROLE_DEVICE']);
	expect(calls[0].scopeId).toBe('DG_PROD');
	expect(String(calls[0].scopeKind)).toMatch(/DEVICE_GROUP|^1$/);
});

test('USER-kind role offers a user-group picker and assigns with USER_GROUP scope', async ({ page }) => {
	await preparePage(page, 'light');
	await installScopeFixtures(page);
	await page.route('**/cadestro.v1.ControlService/AssignRoleToUser', (r) => json(r, {}));
	const calls = recordRpc<AssignReq>(page, 'AssignRoleToUser');

	await gotoAndSettle(page, `/users/${USER_ID}`, 'text=Assign Role');
	await openDialogAndSelect(page, 'User Scoped Role');

	await expect(scopePicker(page)).toBeEnabled();
	await chooseScope(page, 'Admins');

	await page.getByRole('dialog').getByRole('button', { name: 'Assign', exact: true }).click();

	await expect.poll(() => calls.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(calls[0].scopeId).toBe('UG_ADMINS');
	expect(String(calls[0].scopeKind)).toMatch(/USER_GROUP|^2$/);
});

test('non-scopable role offers no picker and sends an unscoped grant', async ({ page }) => {
	await preparePage(page, 'light');
	await installScopeFixtures(page);
	await page.route('**/cadestro.v1.ControlService/AssignRoleToUser', (r) => json(r, {}));
	const calls = recordRpc<AssignReq>(page, 'AssignRoleToUser');

	await gotoAndSettle(page, `/users/${USER_ID}`, 'text=Assign Role');
	await openDialogAndSelect(page, 'Org Wide Role');

	// The control stays visible to explain that the role is organization-wide,
	// but it cannot accept a narrower scope.
	await expect(scopePicker(page)).toBeDisabled();

	await page.getByRole('dialog').getByRole('button', { name: 'Assign', exact: true }).click();

	await expect.poll(() => calls.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(calls[0].roleIds).toEqual(['ROLE_ORG']);
	// UNSPECIFIED scope is the proto default → omitted from the JSON request.
	expect(calls[0].scopeId ?? '').toBe('');
	expect(String(calls[0].scopeKind ?? '')).toMatch(/UNSPECIFIED|^0$|^$/);
});

// ---------------------------------------------------------------------------
// Same affordance on the user-group → roles flow.
// ---------------------------------------------------------------------------

const GROUP_ID = '01J6XYZSHOWCASEGROUP0001';

test('user-group flow: DEVICE-kind role assigns to the group with DEVICE_GROUP scope', async ({ page }) => {
	await preparePage(page, 'light');
	await installScopeFixtures(page);
	await page.route('**/cadestro.v1.ControlService/AssignRoleToUserGroup', (r) => json(r, {}));
	const calls = recordRpc<AssignReq>(page, 'AssignRoleToUserGroup');

	await gotoAndSettle(page, `/user-groups/${GROUP_ID}`, 'text=Assign Role');
	await openDialogAndSelect(page, 'Device Scoped Role');

	await expect(scopePicker(page)).toBeEnabled();
	await chooseScope(page, 'Production');

	await page.getByRole('dialog').getByRole('button', { name: 'Assign', exact: true }).click();

	await expect.poll(() => calls.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(calls[0].scopeId).toBe('DG_PROD');
	expect(String(calls[0].scopeKind)).toMatch(/DEVICE_GROUP|^1$/);
});

test('user-group flow: non-scopable role offers no picker and sends an unscoped grant', async ({ page }) => {
	await preparePage(page, 'light');
	await installScopeFixtures(page);
	await page.route('**/cadestro.v1.ControlService/AssignRoleToUserGroup', (r) => json(r, {}));
	const calls = recordRpc<AssignReq>(page, 'AssignRoleToUserGroup');

	await gotoAndSettle(page, `/user-groups/${GROUP_ID}`, 'text=Assign Role');
	await openDialogAndSelect(page, 'Org Wide Role');

	await expect(scopePicker(page)).toBeDisabled();
	await page.getByRole('dialog').getByRole('button', { name: 'Assign', exact: true }).click();

	await expect.poll(() => calls.length, { timeout: 5000 }).toBeGreaterThan(0);
	expect(calls[0].scopeId ?? '').toBe('');
	expect(String(calls[0].scopeKind ?? '')).toMatch(/UNSPECIFIED|^0$|^$/);
});
