// B3 guard rails, exercised through the REAL detail pages.
//
// What is actually at stake here is the query STRING: the chips are a drawing
// of it, the pill's caption is a copy of it, and the save RPC is the only place
// it becomes policy. So every assertion below pins the string, the RPC, or the
// gate in front of the RPC — never the chip markup on its own.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
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

// Only the client is faked — generated protobuf re-exports stay real so the
// pages' enums (DeviceStatus, RoleGrantScopeKind) are the production ones.
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

const RULE = 'device.os equals "ubuntu" AND device.labels.env equals "production"';

// The two context ids that compete for the single pill slot on this page.
const GROUP_ID = '01HZDEVGRP0000000000000000';
// ONE context per group. The Rule tab used to publish a second one, which is
// why renaming a group AND editing its rule took two separate saves.
const groupContextId = `device-group:${GROUP_ID}`;
/** The first chip's stored value — refilling it makes the query match RULE
 *  again, which is how the rule editor goes clean and lets the slot go. */
const savedValue = 'ubuntu';

function deviceGroup(over: Record<string, unknown> = {}) {
	return {
		id: mocks.params.id,
		name: 'Production Linux',
		description: 'linux fleet',
		memberCount: 2,
		isDynamic: true,
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
		deviceIds: ['dev-1', 'dev-2'],
		devices: [
			{ deviceId: 'dev-1', hostname: 'web-prod-01', agentVersion: '1.2.3' },
			{ deviceId: 'dev-2', hostname: 'web-prod-02', agentVersion: '1.2.3' }
		]
	});
	mocks.listDevices.mockResolvedValue({
		devices: [
			{ id: 'dev-1', hostname: 'web-prod-01', status: 1, labels: { env: 'production' } },
			{ id: 'dev-2', hostname: 'web-prod-02', status: 2, labels: { env: 'production' } }
		],
		nextPageToken: ''
	});
}

/** Open the Rule tab of the already-rendered detail page. */
async function openRuleTab() {
	await browser.getByRole('tab', { name: 'Rule' }).click();
	await vi.waitFor(() => expect(document.querySelector('[data-testid="rule-tab"]')).toBeTruthy());
}

function chips(): HTMLElement[] {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="query-chip"]')];
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

afterEach(() => resetShell());

describe('chip editor — drawing an existing rule', () => {
	it('renders the stored query as one chip per condition, key · operator · value', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		const drawn = chips().map((c) => c.dataset.query);
		expect(drawn).toEqual([
			'device.os equals "ubuntu"',
			'device.labels.env equals "production"'
		]);

		// the join between them is its own control, not part of a chip
		const join = document.querySelector('[data-testid="query-join"]');
		expect(join?.textContent?.trim()).toBe('AND');

		// key / operator / value carry distinct roles, in mono
		const [keySpan, opSpan, valueSpan] = [...chips()[0].querySelectorAll('span')];
		expect(keySpan.textContent).toBe('device.os');
		expect(opSpan.textContent).toBe('equals');
		expect(valueSpan.textContent).toBe('"ubuntu"');
		expect(getComputedStyle(chips()[0]).fontFamily.toLowerCase()).toContain('mono');
	});

	it('draws a parenthesised alternative inside a dashed group', async () => {
		seedDeviceGroup({
			dynamicQuery:
				'device.labels.env equals "production" AND (device.os equals "ubuntu" OR device.os equals "debian")'
		});
		render(DeviceGroupPage);
		await openRuleTab();

		const group = document.querySelector('[data-testid="query-group"]');
		expect(group, 'the OR alternative renders as a group').toBeTruthy();
		expect(getComputedStyle(group!).borderTopStyle).toBe('dashed');
		expect(group!.querySelectorAll('[data-testid="query-chip"]').length).toBe(2);
	});
});

describe('chip editor — edits produce the exact query string', () => {
	it('sends the recompiled string to the save RPC, byte for byte', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await chips()[0].click();
		const valueInput = browser.getByLabelText('Value');
		await valueInput.fill('debian');

		await vi.waitFor(() =>
			expect(mocks.validateDynamicQuery).toHaveBeenCalledWith(
				'device.os equals "debian" AND device.labels.env equals "production"'
			)
		);

		expect(commitContext(), 'the pill commits the rule').toBe(true);
		await browser.getByTestId('future-scope-confirm').click();

		await vi.waitFor(() => expect(mocks.updateDeviceGroupQuery).toHaveBeenCalled());
		expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledWith(
			mocks.params.id,
			true,
			'device.os equals "debian" AND device.labels.env equals "production"'
		);
	});

	it('recompiles a removed condition without leaving a dangling conjunction', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await document.querySelectorAll<HTMLElement>('[data-testid="query-chip-remove"]')[0].click();

		await vi.waitFor(() =>
			expect(mocks.validateDynamicQuery).toHaveBeenCalledWith(
				'device.labels.env equals "production"'
			)
		);
	});

	it('toggling the join swaps AND for OR in the compiled string', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await browser.getByTestId('query-join').click();

		await vi.waitFor(() =>
			expect(mocks.validateDynamicQuery).toHaveBeenCalledWith(
				'device.os equals "ubuntu" OR device.labels.env equals "production"'
			)
		);
	});
});

describe('an unusable draft never reaches the server', () => {
	it('an empty new chip is not yet a rule change: no RPC, no commit surface', async () => {
		render(DeviceGroupPage);
		await openRuleTab();
		mocks.validateDynamicQuery.mockClear();

		await browser.getByTestId('query-add-condition').click();
		// give the debounce more than its own window to misfire in
		await new Promise((resolve) => setTimeout(resolve, 500));

		expect(mocks.validateDynamicQuery).not.toHaveBeenCalled();
		expect(mocks.evaluateDynamicGroup).not.toHaveBeenCalled();
		// The pill is the group's action bar and is held for the whole visit, so
		// what must be absent is anything to COMMIT — not the pill itself.
		expect(shell.pill.context?.id).toBe(groupContextId);
		expect(shell.pill.context?.dirty, 'an empty chip is not a rule change').toBe(false);
		expect(commitContext(), 'nothing to commit yet').toBe(false);
		expect(mocks.updateDeviceGroupQuery).not.toHaveBeenCalled();
	});

	it('a half-typed condition blocks the commit instead of saving a narrower rule', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await chips()[0].click();
		mocks.validateDynamicQuery.mockClear();
		// Clearing the value drops that condition from the compiled string — the
		// rule would silently widen if the editor treated this as committable.
		await browser.getByLabelText('Value').fill('');
		await new Promise((resolve) => setTimeout(resolve, 500));

		expect(mocks.validateDynamicQuery, 'an incomplete rule never reaches the server').not.toHaveBeenCalled();
		expect(mocks.evaluateDynamicGroup).not.toHaveBeenCalled();
		expect(shell.pill.context?.valid, 'an incomplete rule cannot commit').toBe(false);
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

		await chips()[0].click();
		await browser.getByLabelText('Value').fill('debian');

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
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

		await chips()[0].click();
		await browser.getByLabelText('Value').fill('debian');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();

		const dialog = browser.getByTestId('future-scope-dialog');
		await expect.element(dialog).toBeVisible();
		await expect
			.element(browser.getByTestId('future-scope-standing'))
			.toHaveTextContent('New matches apply automatically');
		await expect
			.element(browser.getByTestId('future-scope-query'))
			.toHaveTextContent('device.os equals "debian"');
		expect(mocks.updateDeviceGroupQuery, 'nothing saves before the acknowledgement').not.toHaveBeenCalled();
	});

	it('CANCELLING the confirm saves nothing and hands the draft back to the pill', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await chips()[0].click();
		await browser.getByLabelText('Value').fill('debian');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();
		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		await browser.getByTestId('future-scope-cancel').click();

		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`device-group:${mocks.params.id}`));
		expect(mocks.updateDeviceGroupQuery).not.toHaveBeenCalled();
		// the edit survives the cancel
		expect(chips()[0].dataset.query).toBe('device.os equals "debian"');
	});

	it('says nothing about dropped members when a static group is empty', async () => {
		seedDeviceGroup({ isDynamic: false, dynamicQuery: '', memberCount: 0 });
		render(DeviceGroupPage);
		await openRuleTab();

		await chips()[0].click();
		await browser.getByLabelText('Select property...').click();
		await browser.getByRole('option', { name: 'Hostname' }).click();
		await browser.getByLabelText('Value').fill('web-prod-01');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();
		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		expect(document.querySelector('[data-testid="future-scope-convert-members"]')).toBeNull();
	});

	it('names the conversion when the group is still static', async () => {
		seedDeviceGroup({ isDynamic: false, dynamicQuery: '' });
		render(DeviceGroupPage);
		await openRuleTab();

		await expect
			.element(browser.getByTestId('rule-futurebar'))
			.toHaveTextContent('converts this group to a standing rule');

		await chips()[0].click();
		await browser.getByLabelText('Select property...').click();
		await browser.getByRole('option', { name: 'Hostname' }).click();
		await browser.getByLabelText('Value').fill('web-prod-01');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		commitContext();
		await expect
			.element(browser.getByTestId('future-scope-dialog'))
			.toHaveTextContent('Convert to a standing rule?');
		// The server clears the curated membership in the same transaction that
		// sets the mode, so the confirm has to price that in — naming only the
		// mode change would let an operator convert a hand-picked group without
		// being told its members are about to go.
		await expect
			.element(browser.getByTestId('future-scope-convert-members'))
			.toHaveTextContent('are dropped');
		await browser.getByTestId('future-scope-confirm').click();
		await vi.waitFor(() =>
			expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledWith(
				mocks.params.id,
				true,
				'device.hostname equals "web-prod-01"'
			)
		);
	});
});

describe('the live count rides the pill subtext', () => {
	it('puts the server match count AND the compiled query in the caption, not in the card', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await chips()[0].click();
		await browser.getByLabelText('Value').fill('debian');

		await vi.waitFor(() => {
			expect(shell.pill.context?.subtext).toBe(
				'47 devices match · device.os equals "debian" AND device.labels.env equals "production"'
			);
		});
		expect(shell.pill.context?.subtextTone).toBe('neutral');
		// one copy only — the card carries no second count line
		expect(document.querySelector('[data-testid="query-status"]')).toBeNull();
	});

	it('drops the caption again once the draft matches the stored rule', async () => {
		render(DeviceGroupPage);
		await openRuleTab();

		await chips()[0].click();
		await browser.getByLabelText('Value').fill('debian');
		await vi.waitFor(() => expect(shell.pill.context?.subtext).toBeTruthy());

		await browser.getByLabelText('Value').fill('ubuntu');
		await vi.waitFor(() => {
			// Back to the stored rule: nothing to commit, so the caption goes away.
			// The bar itself stays — it is the group's action bar, not the rule's.
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
				id: '01HZUSRGRP000000000000000',
				name: 'Directory',
				description: '',
				memberCount: 1,
				isDynamic: false,
				dynamicQuery: '',
				isScimManaged: true,
				roleGrants: [],
				maintenanceWindow: undefined,
				...over
			},
			members: [{ userId: 'usr-1', email: 'ada@example.test' }]
		});
	}

	beforeEach(() => {
		mocks.params = { id: '01HZUSRGRP000000000000000' };
		mocks.url = new URL('http://localhost/user-groups/01HZUSRGRP000000000000000');
	});

	/** The group's own actions, wherever they live — the pill is that home now,
	 *  so a DOM query for a trash glyph would no longer prove anything. */
	function pillActionIds(): string[] {
		return (shell.pill.context?.extraActions ?? []).map((a) => a.id);
	}

	it('disables every membership, identity and rule mutation on a SCIM-managed group', async () => {
		seedUserGroup();
		render(UserGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="group-header"]')).toBeTruthy());

		expect(document.querySelector('[aria-label="Edit name and description"]')).toBeNull();
		// Delete moved to the pill, so that is where its absence has to be proven:
		// a SCIM group's lifecycle lives at the identity provider and it must not be
		// offered an action that the next sync would undo.
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
		// The maintenance window is a group-wide policy and rides the pill for both
		// kinds of group; Delete only for the one this control plane owns.
		expect(pillActionIds()).toEqual(['window', 'delete']);
		expect(
			[...document.querySelectorAll('[role="tab"]')].map((t) => t.textContent?.trim())
		).toContain('Rule');
		expect(document.querySelector('[data-testid="add-member"]')).not.toBeNull();
		expect(document.querySelector('[aria-label="Remove member from group"]')).not.toBeNull();
	});

	it('counts users through the user-group validate RPC and gates its save the same way', async () => {
		seedUserGroup({ isScimManaged: false, isDynamic: true, dynamicQuery: 'user.email endsWith "@example.test"' });
		mocks.updateUserGroupQuery.mockResolvedValue(undefined);
		render(UserGroupPage);
		await openRuleTab();

		expect(chips().map((c) => c.dataset.query)).toEqual(['user.email endsWith "@example.test"']);

		await chips()[0].click();
		await browser.getByLabelText('Value').fill('@corp.test');

		await vi.waitFor(() =>
			expect(mocks.validateUserGroupQuery).toHaveBeenCalledWith('user.email endsWith "@corp.test"')
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
	// It is held from the moment the group loads, not only while the identity
	// fields are open: Delete and the maintenance window are the GROUP's actions
	// and cannot live somewhere that appears only after you have typed.
	it('holds the group from load, with its own actions and nothing to commit', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="group-header"]')).toBeTruthy());

		await vi.waitFor(() =>
			expect(shell.pill.context?.id).toBe(`device-group:${mocks.params.id}`)
		);
		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual(['window', 'delete']);
		// nothing edited yet: nothing to save, and nothing worth parking
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
		expect(shell.pill.context?.valid).toBe(true);
	});

	it('commits the name AND the rule from ONE save', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(groupContextId));

		// Rename on the identity surface…
		await browser.getByLabelText('Edit name and description').click();
		const nameField = browser.getByLabelText('Name');
		await nameField.fill('Renamed fleet');

		// …edit the rule on the Rule tab. The bar never changes hands: one entity,
		// one context, one Save. Two contexts meant the operator had to save twice
		// and whichever surface held the slot decided which half was committed.
		await openRuleTab();
		expect(shell.pill.context?.id).toBe(groupContextId);
		await chips()[0].click();
		await browser.getByLabelText('Value').fill('workstation-42');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));

		commitContext();
		// A standing rule still takes its acknowledgement — it gates the one commit.
		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		await browser.getByTestId('future-scope-confirm').click();

		await vi.waitFor(() => expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledTimes(1));
		expect(mocks.renameDeviceGroup, 'the rename rode the same save').toHaveBeenCalledWith(
			mocks.params.id,
			'Renamed fleet'
		);
		expect(mocks.updateDeviceGroupQuery).toHaveBeenCalledWith(
			mocks.params.id,
			true,
			'device.os equals "workstation-42" AND device.labels.env equals "production"'
		);
	});

	it('offers Stash on a rule edit, the same as on a name edit', async () => {
		render(DeviceGroupPage);
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(groupContextId));

		await openRuleTab();
		await chips()[0].click();
		await browser.getByLabelText('Value').fill('workstation-42');
		await vi.waitFor(() => expect(shell.pill.context?.dirty).toBe(true));

		// A context can only be parked when it says where it lives and what to
		// carry. The rule editor's own context declared neither, so a half-written
		// rule was the one buffer on this page that could not be set aside.
		expect(shell.pill.context?.route).toBe(`/device-groups/${mocks.params.id}`);
		const parked = shell.pill.context?.stashPayload?.() as { query?: string } | undefined;
		expect(parked?.query, 'the parked card carries the rule, not just the name').toBe(
			'device.os equals "workstation-42" AND device.labels.env equals "production"'
		);
	});
});
