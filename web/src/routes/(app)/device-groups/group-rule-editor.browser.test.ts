import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser, userEvent } from 'vitest/browser';
import { resetShell, shell, commitContext } from '$lib/shell/shell.svelte';

const mocks = vi.hoisted(() => ({
	params: { id: '01HZDEVGRP0000000000000000' } as Record<string, string>,
	url: new URL('http://localhost/device-groups/01HZDEVGRP0000000000000000'),
	getDeviceGroup: vi.fn(),
	listDevices: vi.fn(),
	validateDynamicQuery: vi.fn(),
	updateDeviceGroupQuery: vi.fn(),
	renameDeviceGroup: vi.fn(),
	evaluateDynamicGroup: vi.fn(),
	getUserGroup: vi.fn(),
	validateUserGroupQuery: vi.fn(),
	updateUserGroupQuery: vi.fn(),
	evaluateDynamicUserGroup: vi.fn(),
	deleteUserGroup: vi.fn(),
	removeUserFromGroup: vi.fn(),
	addUserToGroup: vi.fn(),
	updateUserGroup: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		get params() {
			return mocks.params;
		},
		get url() {
			return mocks.url;
		}
	}
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('$lib/navigation', () => ({ goto: vi.fn() }));

vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			getDeviceGroup: mocks.getDeviceGroup,
			listDevices: mocks.listDevices,
			validateDynamicQuery: mocks.validateDynamicQuery,
			updateDeviceGroupQuery: mocks.updateDeviceGroupQuery,
			evaluateDynamicGroup: mocks.evaluateDynamicGroup,
			renameDeviceGroup: mocks.renameDeviceGroup,
			updateDeviceGroupDescription: vi.fn(),
			deleteDeviceGroup: vi.fn(),
			addDeviceToGroup: vi.fn(),
			removeDeviceFromGroup: vi.fn(),
			setDeviceGroupSyncInterval: vi.fn(),
			setDeviceGroupInventoryInterval: vi.fn(),
			setDeviceGroupMaintenanceWindow: vi.fn(),
			getUserGroup: mocks.getUserGroup,
			validateUserGroupQuery: mocks.validateUserGroupQuery,
			updateUserGroupQuery: mocks.updateUserGroupQuery,
			evaluateDynamicUserGroup: mocks.evaluateDynamicUserGroup,
			updateUserGroup: mocks.updateUserGroup,
			deleteUserGroup: mocks.deleteUserGroup,
			addUserToGroup: mocks.addUserToGroup,
			removeUserFromGroup: mocks.removeUserFromGroup,
			revokeRoleFromUserGroup: vi.fn(),
			assignRoleToUserGroup: vi.fn(),
			listUsers: vi.fn().mockResolvedValue({ users: [], nextPageToken: '' })
		},
		fetchAllPages: async <T>(fetchPage: (size: number, token: string) => Promise<{ items: T[] }>) =>
			(await fetchPage(100, '')).items,
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		formatDuration: () => '0s'
	};
});

import DeviceGroupPage from './[id]/+page.svelte';
import UserGroupPage from '../user-groups/[id]/+page.svelte';

const RULE = 'device.os == "ubuntu" && "env" in device.labels && device.labels["env"] == "production"';

const GROUP_ID = '01HZDEVGRP0000000000000000';

const groupContextId = `device-group:${GROUP_ID}`;
const savedValue = 'ubuntu';

function deviceGroup(over: Record<string, unknown> = {}) {
	return {
		id: { value: mocks.params.id },
		name: 'Production Linux',
		description: 'linux fleet',
		memberCount: 2,
		dynamicQuery: RULE,
		syncIntervalMinutes: 0,
		inventoryIntervalMinutes: 0,
		maintenanceWindow: undefined,
		...over
	};
}

function seedDeviceGroup(over: Record<string, unknown> = {}) {
	mocks.getDeviceGroup.mockResolvedValue({
		group: deviceGroup(over),
		deviceIds: [{ value: 'dev-1' }, { value: 'dev-2' }],
		devices: [
			{ deviceId: { value: 'dev-1' }, hostname: 'web-prod-01', agentVersion: '1.2.3' },
			{ deviceId: { value: 'dev-2' }, hostname: 'web-prod-02', agentVersion: '1.2.3' }
		]
	});
	mocks.listDevices.mockResolvedValue({
		devices: [
			{ id: { value: 'dev-1' }, hostname: 'web-prod-01', status: 1, labels: { env: 'production' } },
			{ id: { value: 'dev-2' }, hostname: 'web-prod-02', status: 2, labels: { env: 'production' } }
		],
		nextPageToken: ''
	});
}

async function openRuleTab() {
	await browser.getByRole('tab', { name: 'Rule' }).click();
	await vi.waitFor(() => expect(document.querySelector('[data-testid="rule-tab"]')).toBeTruthy());
}

beforeEach(() => {
	document.body.innerHTML = '';
	resetShell();
	vi.clearAllMocks();
	mocks.params = { id: '01HZDEVGRP0000000000000000' };
	seedDeviceGroup();
	mocks.validateDynamicQuery.mockResolvedValue({
		valid: true,
		error: '',
		matchingDeviceCount: 47
	});
	mocks.validateUserGroupQuery.mockResolvedValue({ valid: true, error: '', matchingUserCount: 9 });
	mocks.updateDeviceGroupQuery.mockResolvedValue(deviceGroup());
});

describe('raw CEL editor', () => {
	it('preserves the stored query and validates raw edits', async () => {
		render(DeviceGroupPage);
		await openRuleTab();
		const input = document.querySelector<HTMLTextAreaElement>('#query-editor-text')!;
		expect(input.value).toBe(RULE);
		input.value = 'device.os == "debian" && "env" in device.labels && device.labels["env"] == "production"';
		input.dispatchEvent(new Event('input', { bubbles: true }));
		await vi.waitFor(() =>
			expect(mocks.validateDynamicQuery).toHaveBeenCalledWith(
				'device.os == "debian" && "env" in device.labels && device.labels["env"] == "production"'
			)
		);
	});

	it('does not let an older validation response overwrite newer text', async () => {
		let resolveFirst: ((result: { valid: boolean; error: string; matchingDeviceCount: number }) => void) | undefined;
		mocks.validateDynamicQuery.mockImplementation((query: string) => {
			if (query.includes('first')) {
				return new Promise((resolve) => {
					resolveFirst = resolve;
				});
			}
			return Promise.resolve({ valid: true, error: '', matchingDeviceCount: 2 });
		});
		render(DeviceGroupPage);
		await openRuleTab();
		mocks.validateDynamicQuery.mockClear();
		const input = browser.getByTestId('query-input');
		await input.fill('device.hostname == "first"');
		await vi.waitFor(() => expect(mocks.validateDynamicQuery).toHaveBeenCalledWith('device.hostname == "first"'));
		await input.fill('device.hostname == "second"');
		await vi.waitFor(() => expect(shell.pill.context?.subtext).toContain('2 devices match'));
		resolveFirst?.({ valid: false, error: 'stale', matchingDeviceCount: 0 });
		await vi.waitFor(() => expect(shell.pill.context?.subtext).toContain('device.hostname == "second"'));
		expect(shell.pill.context?.subtext).not.toContain('stale');
	});
});

describe('an unusable draft never reaches the server', () => {
	it('an empty query is not yet a rule change: no RPC, no commit surface', async () => {
		render(DeviceGroupPage);
		await openRuleTab();
		mocks.validateDynamicQuery.mockClear();

		await browser.getByTestId('query-input').fill('');
		await new Promise((resolve) => setTimeout(resolve, 500));

		expect(mocks.validateDynamicQuery).not.toHaveBeenCalled();
		expect(mocks.evaluateDynamicGroup).not.toHaveBeenCalled();

		expect(shell.pill.context?.id).toBe(groupContextId);
		expect(shell.pill.context?.dirty, 'clearing the query is an invalid edit').toBe(true);
		expect(commitContext(), 'nothing to commit yet').toBe(false);
		expect(mocks.updateDeviceGroupQuery).not.toHaveBeenCalled();
	});

	it('a half-typed condition blocks the commit instead of saving a narrower rule', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await browser.getByTestId('query-input').fill('device.os ==');
		mocks.validateDynamicQuery.mockResolvedValue({ valid: false, error: 'invalid CEL', matchingDeviceCount: 0 });
		await vi.waitFor(() => expect(mocks.validateDynamicQuery).toHaveBeenCalledWith('device.os =='));

		expect(mocks.evaluateDynamicGroup).not.toHaveBeenCalled();
		expect(shell.pill.context?.valid, 'an invalid rule cannot commit').toBe(false);
		expect(commitContext()).toBe(false);
		expect(mocks.updateDeviceGroupQuery).not.toHaveBeenCalled();
	});

	it('keeps the commit shut and evaluate untouched when the server rejects the query', async () => {
		render(DeviceGroupPage);
		await openRuleTab();
		mocks.validateDynamicQuery.mockResolvedValue({
			valid: false,
			error: 'unknown property device.nope',
			matchingDeviceCount: 0
		});

		await browser.getByTestId('query-input').fill('device.os == "debian"');

		await vi.waitFor(() => expect(shell.pill.context?.subtext).toContain('unknown property device.nope'));
		expect(shell.pill.context?.valid).toBe(false);
		expect(mocks.evaluateDynamicGroup).not.toHaveBeenCalled();
		expect(commitContext()).toBe(false);
		expect(mocks.updateDeviceGroupQuery).not.toHaveBeenCalled();
		expect(shell.pill.context?.subtext).toContain('unknown property device.nope');
		expect(shell.pill.context?.subtextTone).toBe('warn');
	});
});

describe('future-scope guard', () => {
	it('gates the save behind a real confirm and states the standing-rule consequence', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await browser.getByTestId('query-input').fill('device.os == "debian" && "env" in device.labels && device.labels["env"] == "production"');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();

		const dialog = browser.getByTestId('future-scope-dialog');
		await expect.element(dialog).toBeVisible();
		await expect
			.element(browser.getByTestId('future-scope-standing'))
			.toHaveTextContent('New matches apply automatically');
		await expect
			.element(browser.getByTestId('future-scope-query'))
			.toHaveTextContent('device.os == "debian"');
		expect(mocks.updateDeviceGroupQuery, 'nothing saves before the acknowledgement').not.toHaveBeenCalled();
	});

	it('CANCELLING the confirm saves nothing and hands the draft back to the pill', async () => {
		render(DeviceGroupPage);
		await openRuleTab();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		await browser.getByTestId('query-input').fill('device.os == "debian"');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();
		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		await browser.getByTestId('future-scope-cancel').click();

		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`device-group:${mocks.params.id}`));
		expect(mocks.updateDeviceGroupQuery).not.toHaveBeenCalled();

		expect(document.querySelector<HTMLTextAreaElement>('#query-editor-text')?.value).toBe('device.os == "debian"');
	});

	it('says nothing about dropped members when a static group is empty', async () => {
		seedDeviceGroup({ dynamicQuery: undefined, memberCount: 0 });
		render(DeviceGroupPage);
		await openRuleTab();

		await browser.getByTestId('query-input').fill('device.hostname == "web-prod-01"');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();
		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		expect(document.querySelector('[data-testid="future-scope-convert-members"]')).toBeNull();
	});

	it('names the conversion when the group is still static', async () => {
		seedDeviceGroup({ dynamicQuery: undefined });
		render(DeviceGroupPage);
		await openRuleTab();

		await expect
			.element(browser.getByTestId('rule-futurebar'))
			.toHaveTextContent('converts this group to a standing rule');

		await browser.getByTestId('query-input').fill('device.hostname == "web-prod-01"');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();
		await expect
			.element(browser.getByTestId('future-scope-dialog'))
			.toHaveTextContent('Convert to a standing rule?');

		await expect
			.element(browser.getByTestId('future-scope-convert-members'))
			.toHaveTextContent('are dropped');
		await browser.getByTestId('future-scope-confirm').click();
		await vi.waitFor(() =>
			expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledWith(
				mocks.params.id,
				'device.hostname == "web-prod-01"'
			)
		);
	});
});

describe('the live count rides the pill subtext', () => {
	it('puts the server match count and query in the caption, not in the card', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await browser.getByTestId('query-input').fill('device.os == "debian"');

		await vi.waitFor(() => {
			expect(shell.pill.context?.subtext).toBe(
				'47 devices match · device.os == "debian"'
			);
		});
		expect(shell.pill.context?.subtextTone).toBe('neutral');

		expect(document.querySelector('[data-testid="query-status"]')).toBeNull();
	});

	it('drops the caption again once the draft matches the stored rule', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await browser.getByTestId('query-input').fill('device.os == "debian"');
		await vi.waitFor(() => expect(shell.pill.context?.subtext).toBeTruthy());

		await browser.getByTestId('query-input').fill(RULE);
		await vi.waitFor(() => {

			expect(shell.pill.context?.subtext).toBeUndefined();
			expect(shell.pill.context?.dirty).toBe(false);
		});
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(groupContextId));
	});
});

describe('user groups — the SCIM guard', () => {
	function seedUserGroup(over: Record<string, unknown> = {}) {
		mocks.getUserGroup.mockResolvedValue({
			group: {
				id: { value: '01HZUSRGRP000000000000000' },
				name: 'Directory',
				description: '',
				memberCount: 1,
				dynamicQuery: undefined,
				isScimManaged: true,
				roleGrants: [],
				maintenanceWindow: undefined,
				...over
			},
			members: [{ userId: { value: 'usr-1' }, email: 'ada@example.test' }]
		});
	}

	beforeEach(() => {
		mocks.params = { id: '01HZUSRGRP000000000000000' };
		mocks.url = new URL('http://localhost/user-groups/01HZUSRGRP000000000000000');
	});

	function pillActionIds(): string[] {
		return (shell.pill.context?.extraActions ?? []).map((a) => a.id);
	}

	it('disables every membership, identity and rule mutation on a SCIM-managed group', async () => {
		seedUserGroup();
		render(UserGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="group-header"]')).toBeTruthy());

		expect(document.querySelector('[aria-label="Edit name and description"]')).toBeNull();

		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`user-group:${mocks.params.id}`));
		expect(pillActionIds()).not.toContain('delete');
		expect(
			[...document.querySelectorAll('[role="tab"]')].map((t) => t.textContent?.trim())
		).not.toContain('Rule');

		expect(document.querySelector('[data-testid="add-member"]')).toBeNull();
		expect(document.querySelector('[aria-label="Remove member from group"]')).toBeNull();
		await expect
			.element(browser.getByTestId('scim-note'))
			.toHaveTextContent('managed by SCIM');
	});

	it('leaves the same surfaces live on a locally-managed group', async () => {
		seedUserGroup({ isScimManaged: false });
		render(UserGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="group-header"]')).toBeTruthy());

		expect(document.querySelector('[aria-label="Edit name and description"]')).not.toBeNull();
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`user-group:${mocks.params.id}`));

		expect(pillActionIds()).toEqual(['window', 'delete']);
		expect(
			[...document.querySelectorAll('[role="tab"]')].map((t) => t.textContent?.trim())
		).toContain('Rule');
		expect(document.querySelector('[data-testid="add-member"]')).not.toBeNull();
		expect(document.querySelector('[aria-label="Remove member from group"]')).not.toBeNull();
	});

	it('counts users through the user-group validate RPC and gates its save the same way', async () => {
		seedUserGroup({ isScimManaged: false, dynamicQuery: 'user.email.endsWith("@example.test")' });
		mocks.updateUserGroupQuery.mockResolvedValue(undefined);
		render(UserGroupPage);
		await openRuleTab();

		expect(document.querySelector<HTMLTextAreaElement>('#query-editor-text')?.value).toBe('user.email.endsWith("@example.test")');

		await browser.getByTestId('query-input').fill('user.email.endsWith("@corp.test")');

		await vi.waitFor(() =>
			expect(mocks.validateUserGroupQuery).toHaveBeenCalledWith('user.email.endsWith("@corp.test")')
		);
		await vi.waitFor(() => expect(shell.pill.context?.subtext).toContain('9 users match'));
		expect(mocks.validateDynamicQuery, 'device validation never runs for a user group').not.toHaveBeenCalled();

		commitContext();
		await browser.getByTestId('future-scope-cancel').click();
		expect(mocks.updateUserGroupQuery).not.toHaveBeenCalled();
	});
});

describe('data-tour anchors', () => {
	it('marks the rule editor, the members list and the header', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-tour="group-header"]')).toBeTruthy());
		expect(document.querySelector('[data-tour="group-members"]')).toBeTruthy();

		await openRuleTab();
		expect(document.querySelector('[data-tour="group-rule-editor"]')).toBeTruthy();
	});
});

describe('the pill is the group’s action bar', () => {

	it('holds the group from load, with its own actions and nothing to commit', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="group-header"]')).toBeTruthy());

		await vi.waitFor(() =>
			expect(shell.pill.context?.id).toBe(`device-group:${mocks.params.id}`)
		);
		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual(['window', 'delete']);

		expect(shell.pill.context?.dirty, 'a clean context has nothing to commit').toBe(false);
		expect(commitContext()).toBe(false);
	});

	it('goes dirty on the identity edit instead of opening a dialog', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="group-header"]')).toBeTruthy());

		await browser.getByLabelText('Edit name and description').click();
		await expect.element(browser.getByTestId('identity-edit')).toBeVisible();

		expect(shell.pill.context?.id).toBe(`device-group:${mocks.params.id}`);
		expect(shell.pill.context?.dirty, 'opening the fields is not an edit').toBe(false);

		await userEvent.fill(browser.getByLabelText('Name'), 'Renamed');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(shell.pill.context?.valid).toBe(true);
	});

	it('commits the name AND the rule from ONE save', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(groupContextId));

		await browser.getByLabelText('Edit name and description').click();
		const nameField = browser.getByLabelText('Name');
		await nameField.fill('Renamed fleet');

		await openRuleTab();
		expect(shell.pill.context?.id).toBe(groupContextId);
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		await browser.getByTestId('query-input').fill('device.os == "workstation-42" && "env" in device.labels && device.labels["env"] == "production"');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		commitContext();

		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		await browser.getByTestId('future-scope-confirm').click();

		await vi.waitFor(() => expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledTimes(1));
		expect(mocks.renameDeviceGroup, 'the rename rode the same save').toHaveBeenCalledWith(
			mocks.params.id,
			'Renamed fleet'
		);
		expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledWith(
			mocks.params.id,
			'device.os == "workstation-42" && "env" in device.labels && device.labels["env"] == "production"'
		);
	});

	it('offers Stash on a rule edit, the same as on a name edit', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(groupContextId));

		await openRuleTab();
		await browser.getByTestId('query-input').fill('device.os == "workstation-42" && "env" in device.labels && device.labels["env"] == "production"');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));

		expect(shell.pill.context?.route).toBe(`/device-groups/${mocks.params.id}`);
		const parked = shell.pill.context?.stashPayload?.() as { query?: string } | undefined;
		expect(parked?.query, 'the parked card carries the rule, not just the name').toBe(
			'device.os == "workstation-42" && "env" in device.labels && device.labels["env"] == "production"'
		);
	});
});
