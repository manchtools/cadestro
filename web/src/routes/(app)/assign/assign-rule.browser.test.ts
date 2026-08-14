// Contract for B3 — targeting by rule from the assign surface.
//
// A rule target is not a new kind of assignment: it is a dynamic device group
// plus an assignment that points at it. So what these tests pin down is exactly
// that mapping — the compiled query string that reaches CreateDeviceGroup, the
// group id that reaches CreateAssignment, the order of the two, and the
// acknowledgement standing in front of both. Everything else (the live count,
// the pill caption, the stash) is asserted against the same string, because the
// string is the rule.
//
// Only `apiClient` is faked. The generated protobuf enums, the paginate helper,
// the query builder, the shell store and the carried-selection store are the
// production modules — they are what the page is being tested against.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { ActionSetSchema, DeviceSchema } from '$contract/cadestro/v1/control_pb';
import {
	AssignmentMode,
	AssignmentSourceType,
	AssignmentTargetType,
	DeviceStatus
} from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';
import {
	shell,
	resetShell,
	commitContext,
	runPillAction,
	stashContext,
	restoreDraft
} from '$lib/shell/shell.svelte';
import { setCarried } from '$lib/shell/carried-selection.svelte';
import { clearAssignDraft } from './draft.svelte';

const DEV_ONLINE = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const SET_PATCH = '01JR0A1E1R7S3T6V0W2X5Y4Z9B';
const GROUP_ID = '01JR0B1K1X2Y3Z4A5B6C7D8E9F';

/** What the chips compile to once the condition below is filled in. */
const QUERY = 'device.hostname equals "web-prod-01"';
const GROUP_NAME = 'production-linux';
const MATCH_COUNT = 47;

const api = vi.hoisted(() => ({
	getDevice: vi.fn(),
	listActionSets: vi.fn(),
	getActionSet: vi.fn(),
	listAssignments: vi.fn(),
	createAssignment: vi.fn(),
	dispatchActionSet: vi.fn(),
	validateDynamicQuery: vi.fn(),
	createDeviceGroup: vi.fn(),
	evaluateDynamicGroup: vi.fn()
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

const SETS = [create(ActionSetSchema, { id: SET_PATCH, name: 'Patch and reboot', memberCount: 2 })];

beforeEach(() => {
	document.body.innerHTML = '';
	resetShell();
	// Assign parks its buffer in a module-state draft (draft.svelte.ts), which
	// resetShell does not touch. Auto-stash-on-navigate runs onStash when a dirty
	// surface unmounts, so a prior test's parked rule/name/set would leak into
	// this one — reset it too for a genuinely clean surface.
	clearAssignDraft();
	setCarried({ deviceIds: [DEV_ONLINE], label: '1 device' });
	for (const fn of Object.values(api)) fn.mockReset();
	nav.goto.mockReset();
	toaster.success.mockReset();
	toaster.error.mockReset();

	api.getDevice.mockImplementation(async (id: string) =>
		create(DeviceSchema, { id, hostname: 'api-prod-01', status: DeviceStatus.ONLINE })
	);
	api.listActionSets.mockResolvedValue({ sets: SETS, nextPageToken: '' });
	api.getActionSet.mockResolvedValue({ set: SETS[0], members: [] });
	api.listAssignments.mockResolvedValue({ assignments: [], nextPageToken: '' });
	api.createAssignment.mockResolvedValue({});
	api.dispatchActionSet.mockResolvedValue([]);
	api.validateDynamicQuery.mockResolvedValue({
		valid: true,
		error: '',
		matchingDeviceCount: MATCH_COUNT
	});
	api.createDeviceGroup.mockResolvedValue({ id: GROUP_ID, name: GROUP_NAME });
});

function context() {
	return shell.pill.context;
}

async function enterRuleMode() {
	await browser.getByRole('radio', { name: m.assign_target_rule() }).click();
	await vi.waitFor(() => expect(document.querySelector('[data-testid="assign-rule-stage"]')).toBeTruthy(), {
		timeout: 3000
	});
}

/** Fill the pane's single empty chip so the editor compiles QUERY. */
async function buildRule() {
	const chip = document.querySelector<HTMLElement>('[data-testid="query-chip"]');
	expect(chip, 'the rule pane opens with one empty condition').toBeTruthy();
	await chip!.click();
	await browser.getByLabelText(m.qb_placeholder_property()).click();
	await browser.getByRole('option', { name: m.qb_prop_hostname() }).click();
	await browser.getByLabelText(m.qb_placeholder_value()).fill('web-prod-01');
	await vi.waitFor(() => expect(api.validateDynamicQuery).toHaveBeenCalledWith(QUERY), {
		timeout: 3000
	});
}

async function chooseSet() {
	await browser.getByRole('radio', { name: /Patch and reboot/ }).click();
}

async function nameGroup(name = GROUP_NAME) {
	await browser.getByTestId('assign-rule-name').fill(name);
}

/** Everything a rule commit needs: a set, a counted rule and a group name. */
async function readyToAssign() {
	await enterRuleMode();
	await buildRule();
	await chooseSet();
	await nameGroup();
	await vi.waitFor(() => expect(context()?.valid).toBe(true), { timeout: 3000 });
}

describe('assign — the target-mode toggle', () => {
	it('swaps the carried stage for the chip rule editor', async () => {
		await render(AssignPage);
		await vi.waitFor(() => expect(document.querySelector('[data-testid="assign-carried-grid"]')).toBeTruthy(), {
			timeout: 3000
		});

		await enterRuleMode();

		expect(document.querySelector('[data-testid="assign-carried-grid"]')).toBeNull();
		expect(document.querySelector('[data-testid="query-editor"]')).toBeTruthy();
		// A rule target has no per-device dispatch on the contract, so the surface
		// offers no schedule it could not honour.
		expect(
			document.querySelector(`[role="radiogroup"][aria-label="${m.assign_schedule_label()}"]`)
		).toBeNull();
		await expect.element(browser.getByTestId('assign-rule-futurebar')).toBeVisible();
	});

	it('goes back to the carried selection with the set choice intact', async () => {
		await render(AssignPage);
		await chooseSet();
		await enterRuleMode();

		await browser.getByRole('radio', { name: m.assign_target_carried() }).click();

		await vi.waitFor(() => expect(document.querySelector('[data-testid="assign-carried-grid"]')).toBeTruthy(), {
			timeout: 3000
		});
		await expect
			.element(browser.getByRole('radio', { name: /Patch and reboot/ }))
			.toHaveAttribute('aria-checked', 'true');
	});
});

describe('assign by rule — the count is the server’s', () => {
	it('rides the validated count and the compiled query in the pill caption', async () => {
		await render(AssignPage);
		await readyToAssign();

		await vi.waitFor(
			() =>
				expect(context()?.subtext).toBe(
					`${m.query_match_count_devices({ count: MATCH_COUNT })} · ${QUERY}`
				),
			{ timeout: 3000 }
		);
		expect(context()?.subtextTone).toBe('neutral');
		// One copy only: the card carries no second count line.
		expect(document.querySelector('[data-testid="query-status"]')).toBeNull();
		// The count that the operator commits against is the one the server gave
		// for THIS query — never Evaluate, which would mutate a group.
		expect(api.validateDynamicQuery).toHaveBeenCalledWith(QUERY);
		expect(api.evaluateDynamicGroup).not.toHaveBeenCalled();
	});

	it('labels the commit with the live match count, not the carried selection', async () => {
		await render(AssignPage);
		await readyToAssign();

		expect(context()?.commitLabel).toBe(m.assign_commit_label({ count: MATCH_COUNT }));
		expect(context()?.title).toBe(m.assign_rule_pill_title());
	});

	it('lists no matching devices, because no RPC lists them for an unsaved rule', async () => {
		await render(AssignPage);
		await enterRuleMode();
		await buildRule();

		const preview = browser.getByTestId('assign-rule-preview');
		await expect
			.element(preview.getByText(m.query_match_count_devices({ count: MATCH_COUNT })))
			.toBeVisible();
		await expect.element(preview.getByText(m.assign_rule_preview_note())).toBeVisible();
		// The count is the ONLY thing claimed. Nothing else on the surface — least
		// of all the carried selection's own hostnames — is dressed up as a match.
		const text = document.querySelector('[data-testid="assign-rule-preview"]')?.textContent ?? '';
		expect(text).not.toContain('api-prod-01');
		expect(text).not.toContain('web-prod-01');
	});

	it('keeps the commit shut and says why when the server rejects the query', async () => {
		api.validateDynamicQuery.mockResolvedValue({
			valid: false,
			error: 'unknown property device.nope',
			matchingDeviceCount: 0
		});

		await render(AssignPage);
		await enterRuleMode();
		await buildRule();
		await chooseSet();
		await nameGroup();

		await vi.waitFor(() => expect(context()?.subtext).toContain('unknown property device.nope'), {
			timeout: 3000
		});
		expect(context()?.valid).toBe(false);
		expect(commitContext()).toBe(false);
		expect(api.createDeviceGroup).not.toHaveBeenCalled();
	});
});

describe('assign by rule — nothing commits until all three are real', () => {
	it('needs a counted rule, a group name AND a set', async () => {
		await render(AssignPage);
		await enterRuleMode();

		await vi.waitFor(() => expect(context()?.id).toBe('assign'), { timeout: 3000 });
		expect(context()?.valid, 'an empty rule pane cannot commit').toBe(false);
		expect(commitContext()).toBe(false);

		await buildRule();
		expect(context()?.valid, 'a counted rule alone is not a target').toBe(false);

		await chooseSet();
		await vi.waitFor(() => expect(context()?.subtext).toContain(m.assign_rule_name_required()), {
			timeout: 3000
		});
		expect(context()?.valid, 'the group still has no name').toBe(false);
		// The store's guard, not just a disabled attribute — ⌘S is closed too.
		expect(commitContext()).toBe(false);

		await nameGroup();
		await vi.waitFor(() => expect(context()?.valid).toBe(true), { timeout: 3000 });
		expect(api.createDeviceGroup, 'nothing was created on the way there').not.toHaveBeenCalled();
		expect(api.createAssignment).not.toHaveBeenCalled();
	});
});

describe('assign by rule — the future-scope acknowledgement', () => {
	it('gates BOTH RPCs behind a real confirm that names the group', async () => {
		await render(AssignPage);
		await readyToAssign();

		expect(commitContext()).toBe(true);

		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		await expect
			.element(browser.getByTestId('future-scope-standing'))
			.toHaveTextContent('New matches apply automatically');
		await expect.element(browser.getByTestId('future-scope-query')).toHaveTextContent(QUERY);
		await expect
			.element(browser.getByTestId('future-scope-note'))
			.toHaveTextContent(GROUP_NAME);
		expect(api.createDeviceGroup, 'nothing is created before the acknowledgement').not.toHaveBeenCalled();
		expect(api.createAssignment).not.toHaveBeenCalled();
	});

	it('CANCELLING creates nothing and hands the draft back to the pill', async () => {
		await render(AssignPage);
		await readyToAssign();

		commitContext();
		await expect.element(browser.getByTestId('future-scope-dialog')).toBeVisible();
		await browser.getByTestId('future-scope-cancel').click();

		await vi.waitFor(() => expect(context()?.id).toBe('assign'), { timeout: 3000 });
		expect(api.createDeviceGroup).not.toHaveBeenCalled();
		expect(api.createAssignment).not.toHaveBeenCalled();
		expect(nav.goto).not.toHaveBeenCalled();
		// the draft survives the cancel
		expect(context()?.valid).toBe(true);
		expect(document.querySelector<HTMLElement>('[data-testid="query-chip"]')?.dataset.query).toBe(
			QUERY
		);
	});
});

describe('assign by rule — the commit is group-create then assignment', () => {
	it('creates the dynamic group with the exact compiled query, then targets it', async () => {
		await render(AssignPage);
		await readyToAssign();

		commitContext();
		await browser.getByTestId('future-scope-confirm').click();

		await vi.waitFor(() => expect(api.createAssignment).toHaveBeenCalled(), { timeout: 3000 });
		expect(api.createDeviceGroup).toHaveBeenCalledTimes(1);
		expect(api.createDeviceGroup).toHaveBeenCalledWith(GROUP_NAME, '', true, QUERY);
		expect(api.createAssignment).toHaveBeenCalledWith(
			AssignmentSourceType.ACTION_SET,
			SET_PATCH,
			AssignmentTargetType.DEVICE_GROUP,
			GROUP_ID,
			AssignmentMode.REQUIRED
		);
		// The assignment can only name a group that already exists.
		expect(api.createDeviceGroup.mock.invocationCallOrder[0]).toBeLessThan(
			api.createAssignment.mock.invocationCallOrder[0]
		);
		// A rule target dispatches nothing: DispatchActionSet is per device.
		expect(api.dispatchActionSet).not.toHaveBeenCalled();
		await vi.waitFor(() => expect(nav.goto).toHaveBeenCalledWith(`/device-groups/${GROUP_ID}`), {
			timeout: 3000
		});
		expect(shell.pill.context).toBeNull();
	});

	it('keeps the created group named when only the assignment fails', async () => {
		api.createAssignment.mockRejectedValue(new Error('backend down'));

		await render(AssignPage);
		await readyToAssign();

		commitContext();
		await browser.getByTestId('future-scope-confirm').click();

		await expect
			.element(browser.getByTestId('assign-rule-error'))
			.toHaveTextContent(GROUP_NAME);
		expect(nav.goto).not.toHaveBeenCalled();
		// A retry assigns THAT group instead of creating a second one for the
		// same rule.
		await vi.waitFor(() => expect(context()?.valid).toBe(true), { timeout: 3000 });
		api.createAssignment.mockResolvedValue({});
		commitContext();
		await browser.getByTestId('future-scope-confirm').click();
		await vi.waitFor(() => expect(nav.goto).toHaveBeenCalledWith(`/device-groups/${GROUP_ID}`), {
			timeout: 3000
		});
		expect(api.createDeviceGroup).toHaveBeenCalledTimes(1);
	});
});

describe('assign by rule — save as group without assigning', () => {
	it('creates only the group and offers a link to it', async () => {
		await render(AssignPage);
		await readyToAssign();

		runPillAction('save-as-group');

		await vi.waitFor(() => expect(api.createDeviceGroup).toHaveBeenCalledTimes(1), { timeout: 3000 });
		expect(api.createDeviceGroup).toHaveBeenCalledWith(GROUP_NAME, '', true, QUERY);
		expect(api.createAssignment, 'saving a group assigns nothing').not.toHaveBeenCalled();
		expect(api.dispatchActionSet).not.toHaveBeenCalled();
		await expect
			.element(browser.getByTestId('assign-rule-saved').getByRole('link'))
			.toHaveAttribute('href', `/device-groups/${GROUP_ID}`);
		// The surface stays put: saving a group is not the assignment.
		expect(nav.goto).not.toHaveBeenCalled();
		expect(context()?.valid).toBe(true);
	});

	it('refuses to create an unnamed or uncounted group', async () => {
		await render(AssignPage);
		await enterRuleMode();
		await buildRule();

		runPillAction('save-as-group');
		await new Promise((resolve) => setTimeout(resolve, 200));

		expect(api.createDeviceGroup).not.toHaveBeenCalled();
	});
});

describe('assign by rule — the third exit', () => {
	it('parks the rule, the name and the set, and restores all three', async () => {
		const first = await render(AssignPage);
		await readyToAssign();

		const draftId = stashContext();

		expect(draftId).toBe('draft:assign');
		expect(shell.drafts[0].subtitle).toBe(m.assign_rule_stash_subtitle({ name: GROUP_NAME }));
		await vi.waitFor(() => expect(shell.pill.context).toBeNull(), { timeout: 3000 });

		await first.unmount();
		// Cross-route restore: the store hands the home route to the chrome and
		// pops the card, staging the buffer for the remount to claim.
		expect(restoreDraft(draftId!)).toBe('/assign');
		expect(shell.drafts).toHaveLength(0);

		await render(AssignPage);
		expect(shell.drafts).toHaveLength(0);

		// Same mode, same rule, same name, same set — and the pill can commit it.
		await vi.waitFor(() => expect(document.querySelector('[data-testid="assign-rule-stage"]')).toBeTruthy(), {
			timeout: 3000
		});
		expect(document.querySelector<HTMLElement>('[data-testid="query-chip"]')?.dataset.query).toBe(
			QUERY
		);
		await expect.element(browser.getByTestId('assign-rule-name')).toHaveValue(GROUP_NAME);
		await expect
			.element(browser.getByRole('radio', { name: /Patch and reboot/ }))
			.toHaveAttribute('aria-checked', 'true');
		await vi.waitFor(() => expect(context()?.valid).toBe(true), { timeout: 3000 });
		expect(context()?.commitLabel).toBe(m.assign_commit_label({ count: MATCH_COUNT }));
	});

	it('drops its context on unmount instead of leaking a stale pill', async () => {
		const result = await render(AssignPage);
		await enterRuleMode();
		await vi.waitFor(() => expect(context()?.id).toBe('assign'), { timeout: 3000 });

		await result.unmount();

		expect(shell.pill.context).toBeNull();
	});
});
