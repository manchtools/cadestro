// Contract for the assign surface (B2).
//
// What these tests pin down is the seam between three real things: the carried
// selection the fleet writes, the RPCs the control service actually exposes,
// and the shell's context pill. Nothing about eligibility is hardcoded in the
// page — the rows have to fall out of a device read and an assignment read, so
// the tests feed those two reads and assert the rows, the caption and the
// commit all agree.
//
// Only `apiClient` is faked. The generated protobuf enums, the paginate helper,
// the shell store and the carried-selection store are the production modules,
// because they are exactly what the page is being tested against.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import {
	ActionSetSchema,
	ActionSetMemberSchema,
	AssignmentSchema,
	DeviceSchema
} from '$contract/cadestro/v1/control_pb';
import {
	AssignmentMode,
	AssignmentSourceType,
	AssignmentTargetType,
	DeviceStatus
} from '$contract/cadestro/v1/common_pb';
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import * as m from '$lib/paraglide/messages';
import {
	shell,
	resetShell,
	commitContext,
	stashContext,
	restoreDraft
} from '$lib/shell/shell.svelte';
import { getCarried, setCarried } from '$lib/shell/carried-selection.svelte';
import { clearAssignDraft } from './draft.svelte';

const DEV_ONLINE_A = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const DEV_ONLINE_B = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const DEV_ASSIGNED = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';
const DEV_OFFLINE = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';
const CARRIED_IDS = [DEV_ONLINE_A, DEV_ONLINE_B, DEV_ASSIGNED, DEV_OFFLINE];

const SET_PATCH = '01JR0A1E1R7S3T6V0W2X5Y4Z9B';
const SET_HARDEN = '01JR0A2F2S8T4V7W1X3Y6Z5A0C';
const ACTION_UPDATE = '01JR0A3G3T9V5W8X2Y4Z7A6B1D';
const ACTION_KERNEL_UPDATE = '01JR0A4H4V0W6X9Y3Z5A8B7C2E';

const api = vi.hoisted(() => ({
	getDevice: vi.fn(),
	listActionSets: vi.fn(),
	getActionSet: vi.fn(),
	listAssignments: vi.fn(),
	createAssignment: vi.fn(),
	dispatchActionSet: vi.fn()
}));

const nav = vi.hoisted(() => ({ goto: vi.fn() }));
const toaster = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return { ...actions, ...control, ...common, apiClient: api };
});

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: nav.goto,
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('svelte-sonner', () => ({ toast: toaster }));

import AssignPage from './+page.svelte';

function device(id: string, hostname: string, status: DeviceStatus) {
	return create(DeviceSchema, { id, hostname, status });
}

const DEVICES: Record<string, ReturnType<typeof device>> = {
	[DEV_ONLINE_A]: device(DEV_ONLINE_A, 'api-prod-01', DeviceStatus.ONLINE),
	[DEV_ONLINE_B]: device(DEV_ONLINE_B, 'api-prod-02', DeviceStatus.ONLINE),
	[DEV_ASSIGNED]: device(DEV_ASSIGNED, 'api-prod-03', DeviceStatus.ONLINE),
	[DEV_OFFLINE]: device(DEV_OFFLINE, 'kiosk-muc-11', DeviceStatus.OFFLINE)
};

const SETS = [
	create(ActionSetSchema, { id: SET_PATCH, name: 'Patch and reboot', memberCount: 2 }),
	create(ActionSetSchema, { id: SET_HARDEN, name: 'Harden SSH baseline', memberCount: 4 })
];

beforeEach(() => {
	document.body.innerHTML = '';
	resetShell();
	// Assign parks its buffer in a module-state draft (draft.svelte.ts), which
	// resetShell does not touch. Auto-stash-on-navigate now runs onStash on
	// unmount, so a prior test's parked assignment would leak into this one —
	// reset it too for a genuinely clean surface.
	clearAssignDraft();
	setCarried({ deviceIds: [...CARRIED_IDS], label: '4 devices' });
	for (const fn of Object.values(api)) fn.mockReset();
	nav.goto.mockReset();
	toaster.success.mockReset();
	toaster.error.mockReset();

	api.getDevice.mockImplementation(async (id: string) => DEVICES[id]);
	api.listActionSets.mockResolvedValue({ sets: SETS, nextPageToken: '' });
	api.getActionSet.mockResolvedValue({
		set: SETS[0],
		members: [
			create(ActionSetMemberSchema, {
				actionId: ACTION_UPDATE,
				sortOrder: 0,
				actionName: 'full system update',
				actionType: ActionType.UPDATE
			}),
			create(ActionSetMemberSchema, {
				actionId: ACTION_KERNEL_UPDATE,
				sortOrder: 1,
				actionName: 'update kernel if changed',
				actionType: ActionType.UPDATE
			})
		]
	});
	// Only DEV_ASSIGNED already carries the set.
	api.listAssignments.mockResolvedValue({
		assignments: [
			create(AssignmentSchema, {
				id: '01JR0A5J5W1X7Y0Z4A6B9C8D3F',
				sourceType: AssignmentSourceType.ACTION_SET,
				sourceId: SET_PATCH,
				targetType: AssignmentTargetType.DEVICE,
				targetId: DEV_ASSIGNED
			})
		],
		nextPageToken: ''
	});
	api.createAssignment.mockResolvedValue({});
	api.dispatchActionSet.mockResolvedValue([]);
});

/** The pill context this page owns, once its effect has published it. */
async function contextReady() {
	await vi.waitFor(() => expect(shell.pill.context?.id).toBe('assign'), { timeout: 3000 });
	return shell.pill.context!;
}

function setOption(name: string) {
	return browser.getByRole('radio', { name: new RegExp(name) });
}

async function chooseSet(name = 'Patch and reboot') {
	await setOption(name).click();
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('assign — arriving without a carried selection', () => {
	it('says what to do instead of rendering an empty stage', async () => {
		setCarried(null);

		await render(AssignPage);

		await expect.element(browser.getByText(m.assign_empty_title())).toBeVisible();
		await expect.element(browser.getByText(m.assign_empty_hint())).toBeVisible();
		expect(api.getDevice).not.toHaveBeenCalled();
	});

	it('never takes the pill hostage for a selection that does not exist', async () => {
		setCarried(null);

		await render(AssignPage);

		expect(shell.pill.context).toBeNull();
	});
});

describe('assign — eligibility is derived from the two real reads', () => {
	it('reads status for exactly the carried ids', async () => {
		await render(AssignPage);

		await vi.waitFor(() => expect(api.getDevice).toHaveBeenCalledTimes(CARRIED_IDS.length), {
			timeout: 3000
		});
		expect(api.getDevice.mock.calls.map((c) => c[0]).sort()).toEqual([...CARRIED_IDS].sort());
	});

	it('splits the selection into apply-now, update-in-place and queued', async () => {
		await render(AssignPage);
		await contextReady();
		await chooseSet();

		// One ACTION_SET→DEVICE listing answers "who already has this set".
		await vi.waitFor(
			() =>
				expect(api.listAssignments).toHaveBeenCalledWith(
					expect.any(Number),
					'',
					AssignmentSourceType.ACTION_SET,
					SET_PATCH,
					AssignmentTargetType.DEVICE,
					''
				),
			{ timeout: 3000 }
		);

		const rows = browser.getByTestId('assign-eligibility');
		// online, not yet assigned → apply now
		await expect.element(rows.getByText(m.assign_eligibility_ready({ count: 2 }))).toBeVisible();
		// already assigned → update in place, whatever its connectivity
		await expect.element(rows.getByText(m.assign_eligibility_update({ count: 1 }))).toBeVisible();
		// offline → queued, never "skipped": the server keeps the delivery durable
		await expect.element(rows.getByText(m.assign_eligibility_queued({ count: 1 }))).toBeVisible();
	});

	it('reports an unreadable device as unknown, not as offline', async () => {
		api.getDevice.mockImplementation(async (id: string) => {
			if (id === DEV_ONLINE_B) throw new Error('gone');
			return DEVICES[id];
		});

		await render(AssignPage);
		await contextReady();
		await chooseSet();

		const rows = browser.getByTestId('assign-eligibility');
		await expect.element(rows.getByText(m.assign_eligibility_ready({ count: 1 }))).toBeVisible();
		await expect.element(rows.getByText(m.assign_eligibility_queued({ count: 1 }))).toBeVisible();
		await expect.element(rows.getByText(m.assign_eligibility_unknown({ count: 1 }))).toBeVisible();
	});

	it('does not read an unreadable assignment list as "nobody has this set"', async () => {
		api.listAssignments.mockRejectedValue(new Error('backend down'));

		await render(AssignPage);
		await contextReady();
		await chooseSet();

		// The rollup cannot claim an update count it never read — the failure is
		// on screen rather than swallowed into a confident zero.
		await expect.element(browser.getByText(m.assign_load_failed())).toBeVisible();
	});

	it('rides the same rollup into the pill caption', async () => {
		await render(AssignPage);
		await contextReady();
		await chooseSet();

		await vi.waitFor(
			() =>
				expect(shell.pill.context?.subtext).toBe(
					m.assign_caption({ set: 'Patch and reboot', ready: 2, update: 1, queued: 1 })
				),
			{ timeout: 3000 }
		);
	});

	it('expands the chosen set to its real steps', async () => {
		await render(AssignPage);
		await contextReady();
		await chooseSet();

		const steps = browser.getByTestId('assign-set-steps');
		// Sort order, the app's own type label, the action's real name — and
		// The action type must not degrade to "UNSPECIFIED" the way the SDK's own
		// enum-to-string helper does.
		await expect
			.element(steps.getByText(`1 · ${m.actions_type_update()} · full system update`))
			.toBeVisible();
		await expect
			.element(steps.getByText(`2 · ${m.actions_type_update()} · update kernel if changed`))
			.toBeVisible();
	});
});

describe('assign — the pill is the only commit surface', () => {
	it('cannot commit until a set is chosen', async () => {
		await render(AssignPage);
		const ctx = await contextReady();

		expect(ctx.valid).toBe(false);
		expect(ctx.title).toBe(m.assign_pill_title({ label: '4 devices' }));
		expect(ctx.commitLabel).toBe(m.assign_commit_label({ count: 4 }));
		expect(ctx.subtext).toBe(m.assign_caption_choose());
		// The store's guard, not just a disabled attribute — ⌘S is closed too.
		expect(commitContext()).toBe(false);
		expect(api.createAssignment).not.toHaveBeenCalled();

		await chooseSet();
		expect(shell.pill.context?.valid).toBe(true);
	});

	it('assigns and dispatches exactly the carried ids', async () => {
		await render(AssignPage);
		await contextReady();
		await chooseSet();

		expect(commitContext()).toBe(true);

		await vi.waitFor(
			() => expect(api.createAssignment).toHaveBeenCalledTimes(CARRIED_IDS.length),
			{ timeout: 3000 }
		);
		for (const id of CARRIED_IDS) {
			expect(api.createAssignment).toHaveBeenCalledWith(
				AssignmentSourceType.ACTION_SET,
				SET_PATCH,
				AssignmentTargetType.DEVICE,
				id,
				AssignmentMode.REQUIRED
			);
			expect(api.dispatchActionSet).toHaveBeenCalledWith(id, SET_PATCH);
		}
		expect(api.createAssignment.mock.calls.map((c) => c[3]).sort()).toEqual(
			[...CARRIED_IDS].sort()
		);
	});

	it('assigns without dispatching when the schedule is the set’s own', async () => {
		await render(AssignPage);
		await contextReady();
		await chooseSet();

		await browser.getByRole('radio', { name: m.assign_schedule_on_schedule() }).click();
		expect(commitContext()).toBe(true);

		await vi.waitFor(
			() => expect(api.createAssignment).toHaveBeenCalledTimes(CARRIED_IDS.length),
			{ timeout: 3000 }
		);
		expect(api.dispatchActionSet).not.toHaveBeenCalled();
	});

	it('clears the carried selection and hands over to executions on success', async () => {
		await render(AssignPage);
		await contextReady();
		await chooseSet();

		commitContext();

		await vi.waitFor(() => expect(nav.goto).toHaveBeenCalledWith('/executions'), { timeout: 3000 });
		expect(getCarried()).toBeNull();
		expect(toaster.success).toHaveBeenCalledWith(m.assign_commit_success({ count: 4 }));
		expect(shell.pill.context).toBeNull();
	});

	it('names the devices that failed and keeps the selection for a retry', async () => {
		api.createAssignment.mockImplementation(async (...args: unknown[]) => {
			if (args[3] === DEV_OFFLINE) throw new Error('boom');
			return {};
		});

		await render(AssignPage);
		await contextReady();
		await chooseSet();

		commitContext();

		await expect
			.element(browser.getByTestId('assign-failures').getByText(new RegExp(DEV_OFFLINE)))
			.toBeVisible();
		expect(toaster.error).toHaveBeenCalledWith(m.assign_commit_partial({ ok: 3, failed: 1 }));
		expect(nav.goto).not.toHaveBeenCalled();
		expect(getCarried()?.deviceIds).toEqual(CARRIED_IDS);
		// The pill comes back so the operator can retry from the same place.
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
	});
});

describe('assign — the third exit', () => {
	it('parks the draft and restores the same selection and set', async () => {
		const first = await render(AssignPage);
		await contextReady();
		await chooseSet();

		const draftId = stashContext();

		expect(draftId).toBe('draft:assign');
		expect(shell.drafts).toHaveLength(1);
		expect(shell.drafts[0].subtitle).toBe(
			m.assign_stash_subtitle({ set: 'Patch and reboot' })
		);
		// Parked means parked: the pill is free and does not re-enter itself.
		await vi.waitFor(() => expect(shell.pill.context).toBeNull(), { timeout: 3000 });

		// The operator navigates away, then clicks the stage card. The store hands
		// the surface's home route back to the chrome instead of re-entering a
		// context whose closures point at an unmounted page.
		await first.unmount();
		expect(restoreDraft(draftId!)).toBe('/assign');
		// …and the card leaves the rail on the click; the buffer is staged for the
		// remount to claim.
		expect(shell.drafts).toHaveLength(0);

		await render(AssignPage);
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(shell.pill.context?.commitLabel).toBe(m.assign_commit_label({ count: 4 }));
		expect(getCarried()?.deviceIds).toEqual(CARRIED_IDS);
		await expect.element(setOption('Patch and reboot')).toHaveAttribute('aria-checked', 'true');
	});

	it('drops its context on unmount instead of leaking a stale pill', async () => {
		const result = await render(AssignPage);
		await contextReady();

		await result.unmount();

		expect(shell.pill.context).toBeNull();
	});
});
