

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { RoleSchema, PermissionInfoSchema } from '$contract/cadestro/v1/control_pb';
import { PermissionTargetKind } from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';
import {
	shell,
	resetShell,
	pillMode,
	commitContext,
	requestCancelContext,
	confirmCancelContext,
	stashContext,
	restoreDraft,
	claimDraft,
	setShellPath
} from '$lib/shell/shell.svelte';
import { groupPermissions } from './permission-groups';

const ROLE_ID = vi.hoisted(() => '01JR0A1B2C3D4E5F6G7H8J9K0L');

const api = vi.hoisted(() => ({
	getRole: vi.fn(),
	listPermissions: vi.fn(),
	updateRole: vi.fn(),
	deleteRole: vi.fn()
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock('svelte-sonner', () => ({ toast }));
vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$lib/navigation', () => ({ goto: vi.fn(), pushState: vi.fn(), replaceState: vi.fn() }));
vi.mock('$app/state', () => ({
	page: { url: new URL(`https://control.test/roles/${ROLE_ID}`), params: { id: ROLE_ID } }
}));
vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: '01JR0A000000000000000000AA' }, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn()
	};
});

import RoleEditorPage from './+page.svelte';

const PERMISSIONS = [
	create(PermissionInfoSchema, {
		key: 'ListDevices',
		group: 'Devices',
		description: 'List all devices',
		targetKind: PermissionTargetKind.DEVICE
	}),
	create(PermissionInfoSchema, {
		key: 'SetDeviceLabel',
		group: 'Devices',
		description: 'Set device labels',
		targetKind: PermissionTargetKind.UNSPECIFIED
	}),
	create(PermissionInfoSchema, {
		key: 'ListUsers',
		group: 'Users',
		description: 'List all users',
		targetKind: PermissionTargetKind.USER
	}),
	create(PermissionInfoSchema, {
		key: 'CreateRole',
		group: 'Roles',
		description: 'Create roles',
		targetKind: PermissionTargetKind.UNSPECIFIED
	})
];

const role = () =>
	create(RoleSchema, {
		id: { value: ROLE_ID },
		name: 'Operator',
		description: 'Day-to-day fleet work',
		permissions: ['ListDevices'],
		isSystem: false
	});

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	api.getRole.mockResolvedValue({ role: role() });
	api.listPermissions.mockResolvedValue({ permissions: PERMISSIONS });
	api.updateRole.mockImplementation(async (_id, name, description, permissions) =>
		create(RoleSchema, { id: { value: ROLE_ID }, name, description, permissions, isSystem: false })
	);
});

async function mount() {

	setShellPath(`/roles/${ROLE_ID}`);
	render(RoleEditorPage);
	await vi.waitFor(() => expect(api.listPermissions).toHaveBeenCalled(), { timeout: 3000 });
	await expect.element(browser.getByTestId('roles-matrix')).toBeVisible();
}

const checkbox = (key: string) => browser.getByRole('checkbox', { name: key, exact: true });

describe('role matrix — groups are discovered from ListPermissions', () => {
	it('renders one group per domain the permission list actually carries', async () => {

		expect(PERMISSIONS.length).toBeGreaterThan(0);
		const expectedGroups = groupPermissions(PERMISSIONS).map((g) => g.name);
		expect(expectedGroups).toEqual(['Devices', 'Users', 'Roles']);

		await mount();

		for (const name of expectedGroups) {
			await expect
				.element(browser.getByLabelText(m.roles_matrix_group_toggle({ group: name })))
				.toBeVisible();
		}

		await vi.waitFor(() =>
			expect(document.querySelectorAll('[data-testid="matrix-row"]')).toHaveLength(
				PERMISSIONS.length
			)
		);
	});

	it('falls back to the key prefix when the server labels no group', () => {
		const ungrouped = [
			create(PermissionInfoSchema, { key: 'GetUser:self', group: '', description: '' }),
			create(PermissionInfoSchema, { key: 'GetUser', group: '', description: '' })
		];
		expect(groupPermissions(ungrouped).map((g) => g.name)).toEqual(['GetUser']);
	});

	it('states the failure instead of rendering an empty matrix when no permission comes back', async () => {
		api.listPermissions.mockResolvedValue({ permissions: [] });
		render(RoleEditorPage);

		await expect.element(browser.getByTestId('roles-matrix-unavailable')).toBeVisible();
		await expect.element(browser.getByText(m.roles_matrix_unavailable())).toBeVisible();
		expect(document.querySelectorAll('[data-testid="matrix-row"]')).toHaveLength(0);
	});
});

describe('role matrix — the commit rides the context pill', () => {
	it('has no save button on the card', async () => {
		await mount();
		expect(browser.getByRole('button', { name: m.roles_save() }).elements()).toHaveLength(0);
		expect(browser.getByRole('button', { name: m.roles_editor_commit() }).elements()).toHaveLength(
			0
		);
	});

	it('holds the role from load and goes dirty on the first toggle', async () => {
		await mount();
		await vi.waitFor(() => expect(pillMode()).toBe('context'));
		expect(shell.pill.context?.id).toBe(`role:${ROLE_ID}`);

		expect(shell.pill.context?.dirty).toBe(false);
		expect(commitContext()).toBe(false);

		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toContain('delete');

		await checkbox('ListUsers').click();

		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));
		expect(pillMode()).toBe('context');
		expect(shell.pill.context?.valid).toBe(true);
	});

	it('commits the exact permission set the matrix shows', async () => {
		await mount();

		await checkbox('ListUsers').click();
		await checkbox('CreateRole').click();
		await checkbox('ListDevices').click();

		await vi.waitFor(() => expect(pillMode()).toBe('context'));
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.updateRole).toHaveBeenCalledTimes(1));
		const [id, name, description, permissions] = api.updateRole.mock.calls[0];
		expect(id).toBe(ROLE_ID);
		expect(name).toBe('Operator');
		expect(description).toBe('Day-to-day fleet work');
		expect([...permissions].sort()).toEqual(['CreateRole', 'ListUsers']);
	});

	it('stashes the draft to the stage and resumes it with the buffers intact', async () => {
		await mount();

		await checkbox('ListUsers').click();
		await vi.waitFor(() => expect(pillMode()).toBe('context'));

		const draftId = stashContext();
		expect(draftId).toBe(`draft:role:${ROLE_ID}`);

		await vi.waitFor(() => expect(pillMode()).toBe('nav'));
		expect(shell.drafts.map((d) => d.id)).toEqual([draftId]);
		expect(api.updateRole).not.toHaveBeenCalled();

		expect(restoreDraft(draftId!)).toBeNull();
		await vi.waitFor(() => expect(pillMode()).toBe('context'));
		expect(shell.drafts).toHaveLength(0);
		await expect.element(checkbox('ListUsers')).toHaveAttribute('data-state', 'checked');
	});

	it('carries the edit buffer through the restore, so it can be rebuilt from another route', async () => {
		await mount();

		await checkbox('ListUsers').click();
		await vi.waitFor(() => expect(pillMode()).toBe('context'));
		const draftId = stashContext();

		setShellPath('/devices');
		expect(restoreDraft(draftId!)).toBe(`/roles/${ROLE_ID}`);
		expect(pillMode()).toBe('nav');

		expect(shell.drafts).toHaveLength(0);
		const payload = claimDraft(`role:${ROLE_ID}`) as {
			name: string;
			description: string;
			permissions: string[];
		};
		expect(payload.name).toBe('Operator');
		expect(payload.description).toBe('Day-to-day fleet work');
		expect([...payload.permissions].sort()).toEqual(['ListDevices', 'ListUsers']);
	});

	it('releases the pill when the edit is cancelled back to the loaded role', async () => {
		await mount();

		await checkbox('ListUsers').click();
		await vi.waitFor(() => expect(pillMode()).toBe('context'));

		requestCancelContext();
		expect(shell.pill.cancelPending).toBe(true);
		confirmCancelContext();

		await vi.waitFor(() => expect(pillMode()).toBe('nav'));
		await vi.waitFor(() => expect(checkbox('ListUsers').element().dataset.state).toBe('unchecked'));
		expect(api.updateRole).not.toHaveBeenCalled();
	});
});

describe('role matrix — a global-only permission can never be scoped', () => {
	it('marks an unspecified-target permission global-only, not scopable', async () => {
		await mount();

		const globalOnly = browser
			.getByTestId('matrix-row')
			.elements()
			.find((el) => el.getAttribute('data-permission') === 'SetDeviceLabel');
		expect(globalOnly).toBeDefined();
		expect(globalOnly!.querySelector(`[aria-label="${m.roles_matrix_global_only()}"]`)).not.toBeNull();
		expect(globalOnly!.querySelector(`[aria-label="${m.roles_scopable_device()}"]`)).toBeNull();
		expect(globalOnly!.querySelector(`[aria-label="${m.roles_scopable_user()}"]`)).toBeNull();
	});

	it('drops the role to org-wide-only as soon as a global-only permission joins it', async () => {
		await mount();

		const scopability = browser.getByTestId('role-scopability');
		await expect.element(scopability).toBeVisible();
		expect(scopability.element().dataset.scope).toBe('device');

		await checkbox('SetDeviceLabel').click();

		await expect.element(browser.getByText(m.roles_scopability_mixed())).toBeVisible();
		expect(scopability.elements()).toHaveLength(0);
	});
});
