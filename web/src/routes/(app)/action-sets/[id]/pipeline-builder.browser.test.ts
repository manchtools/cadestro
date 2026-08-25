

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import {
	ManagedActionSchema,
	ActionSetSchema,
	ActionSetMemberSchema,
	type ManagedAction,
	type ActionSet,
	type ActionSetMember
} from '$contract/cadestro/v1/control_pb';
import {
	ActionType,
	PackageParamsSchema,
	ServiceParamsSchema
} from '$contract/cadestro/v1/actions_pb';
import * as m from '$lib/paraglide/messages';

const SET_ID = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';
const PKG_ID = '01JQZZ9F2S8T4W7X1Y3Z6A5B0C';
const SVC_ID = '01JQZZAG3T9V5X8Y2Z4A7B6C1D';

const SPARE_ID = '01JQZZBH4U0W6Y9Z3A5B8C7D2E';

const api = vi.hoisted(() => ({
	createAction: vi.fn(),
	addActionToSet: vi.fn(),
	removeActionFromSet: vi.fn(),
	reorderActionInSet: vi.fn(),
	renameActionSet: vi.fn(),
	updateActionSetDescription: vi.fn(),
	updateActionParams: vi.fn(),
	renameAction: vi.fn(),
	updateActionDescription: vi.fn(),
	getActionSet: vi.fn(),
	listActions: vi.fn()
}));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn(),

		useDraft: <T>(_type: string, _id: string, initial: T) => {
			let data = initial;
			return {
				get data() {
					return data;
				},
				set data(next: T) {
					data = next;
				},
				update(partial: Partial<T>) {
					data = { ...data, ...partial };
				},
				clear: async () => {},
				get hasDraft() {
					return false;
				}
			};
		}
	};
});

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('$app/state', () => ({ page: { url: new URL('https://control.test/action-sets') } }));

import ActionSetBuilder from '$lib/components/actions/pipeline/action-set-builder.svelte';
import {
	shell,
	resetShell,
	commitContext,
	runPillAction,
	stashContext,
	restoreDraft,
	setShellPath
} from '$lib/shell/shell.svelte';
import { resetStepKeys } from '$lib/components/actions/pipeline/step-draft';

function packageAction(id: string, name: string, pkg: string): ManagedAction {
	return create(ManagedActionSchema, {
		id: { value: id },
		name,
		description: 'nightly',
		type: ActionType.PACKAGE,
		timeoutSeconds: 300,
		params: { case: 'package', value: create(PackageParamsSchema, { name: pkg }) }
	});
}

function serviceAction(id: string, name: string, unit: string): ManagedAction {
	return create(ManagedActionSchema, {
		id: { value: id },
		name,
		description: '',
		type: ActionType.SERVICE,
		timeoutSeconds: 300,
		params: { case: 'service', value: create(ServiceParamsSchema, { unitName: unit }) }
	});
}

const set: ActionSet = create(ActionSetSchema, {
	id: { value: SET_ID },
	name: 'Harden SSH baseline',
	description: 'baseline'
});

function members(): ActionSetMember[] {
	return [
		create(ActionSetMemberSchema, {
			actionId: { value: SVC_ID },
			sortOrder: 1,
			actionName: 'Enable sshd',
			actionType: ActionType.SERVICE
		}),
		create(ActionSetMemberSchema, {
			actionId: { value: PKG_ID },
			sortOrder: 0,
			actionName: 'Install openssh-server',
			actionType: ActionType.PACKAGE
		})
	];
}

function library(): ManagedAction[] {
	return [
		packageAction(PKG_ID, 'Install openssh-server', 'openssh-server'),
		serviceAction(SVC_ID, 'Enable sshd', 'sshd'),
		packageAction(SPARE_ID, 'Install ripgrep', 'ripgrep')
	];
}

let deleteRuns = 0;

function mount(overrides: { members?: ActionSetMember[] } = {}) {

	setShellPath(`/action-sets/${SET_ID}`);
	return render(ActionSetBuilder, {
		props: {
			setId: SET_ID,
			set,
			members: overrides.members ?? members(),
			library: library(),
			entityActions: [
				{
					id: 'delete',
					label: m.action_sets_delete_action_set(),
					onRun: () => (deleteRuns += 1)
				}
			],
			onsaved: async () => {}
		}
	});
}

function railTitles(): string[] {
	return [...document.querySelectorAll<HTMLElement>('[data-step-key]')].map(
		(el) => el.querySelector('span > span.truncate')?.textContent?.trim() ?? ''
	);
}

function panelInputs(): HTMLInputElement[] {
	return [...document.querySelectorAll<HTMLInputElement>('[data-testid="step-panel"] input')];
}

function panelField(id: string): HTMLInputElement {
	const el = document.querySelector<HTMLInputElement>(`[data-testid="step-panel"] input#${id}`);
	if (!el) throw new Error(`the step panel has no #${id} field`);
	return el;
}

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function paletteNewTab() {
	const tab = document.querySelector<HTMLButtonElement>('[data-palette-tab="new"]');
	if (!tab) throw new Error('the palette never rendered its New tab');
	tab.click();
	await vi.waitFor(() =>
		expect(document.querySelector('[data-palette-entry]')).not.toBeNull()
	);
}

function stepButton(title: string): HTMLButtonElement {
	const found = [...document.querySelectorAll<HTMLButtonElement>('[data-step-key]')].find((el) =>
		el.textContent?.includes(title)
	);
	if (!found) throw new Error(`no pipeline step titled ${title}`);
	return found;
}

beforeEach(() => {
	resetShell();
	resetStepKeys();
	vi.clearAllMocks();
	deleteRuns = 0;
	api.createAction.mockResolvedValue(
		packageAction('01JQZZBH4V0W6Y9Z3A5B8C7D2E', 'Package', 'nginx')
	);
});

describe('the pill is the set’s action bar', () => {
	it('holds the set from mount, clean, carrying the actions the page handed down', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`action-set:${SET_ID}`));

		expect(shell.pill.context?.dirty).toBe(false);
		expect(commitContext()).toBe(false);
		expect(api.renameActionSet).not.toHaveBeenCalled();

		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual(['delete']);
		runPillAction('delete');
		expect(deleteRuns).toBe(1);
		expect(api.removeActionFromSet).not.toHaveBeenCalled();
	});
});

describe('pipeline order', () => {
	it('renders the set as a numbered pipeline in sortOrder, not response order', async () => {
		mount();

		await vi.waitFor(() => expect(railTitles().length).toBe(2));
		expect(railTitles()).toEqual(['Install openssh-server', 'Enable sshd']);

		const numbers = [
			...document.querySelectorAll('[data-tour="builder-pipeline"] [data-step-index]')
		].map((el) => el.textContent?.trim());
		expect(numbers).toEqual(['1', '2']);
	});

	it('anchors the palette, the pipeline and the first step for the product tour', async () => {
		mount();

		await vi.waitFor(() => expect(railTitles().length).toBe(2));
		expect(document.querySelector('[data-tour="builder-palette"]')).not.toBeNull();
		expect(document.querySelector('[data-tour="builder-pipeline"]')).not.toBeNull();
		const first = document.querySelector('[data-tour="builder-step"]');
		expect(first?.textContent).toContain('Install openssh-server');
	});
});

describe('validation blocks the pill commit', () => {
	it('refuses to commit while a step fails its registry schema, and says why', async () => {

		mount({
			members: [
				create(ActionSetMemberSchema, {
					actionId: { value: PKG_ID },
					sortOrder: 0,
					actionName: 'Install openssh-server',
					actionType: ActionType.PACKAGE
				})
			]
		});
		await vi.waitFor(() => expect(railTitles().length).toBe(1));

		await paletteNewTab();
		const newEntry = document.querySelector<HTMLButtonElement>('[data-palette-entry="PACKAGE"]');
		if (!newEntry) throw new Error('the palette never offered the PACKAGE action type');
		newEntry.click();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));
		await vi.waitFor(() => expect(panelInputs().length).toBeGreaterThan(0));

		await vi.waitFor(() => expect(shell.pill.context).not.toBeNull());
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));

		expect(commitContext()).toBe(false);
		expect(api.updateActionParams).not.toHaveBeenCalled();
		expect(shell.pill.context?.subtextTone).toBe('warn');
		expect(shell.pill.context?.subtext).toContain(
			m.action_set_detail_builder_blocked({ count: 1 })
		);

		expect(shell.pill.context?.subtext).toContain('step 2');
		await vi.waitFor(() => expect(document.querySelector('[data-step-error]')).not.toBeNull());
	});

	it('NEVER edits the library action a member points at', async () => {
		mount({
			members: [
				create(ActionSetMemberSchema, {
					actionId: { value: PKG_ID },
					sortOrder: 0,
					actionName: 'Install openssh-server',
					actionType: ActionType.PACKAGE
				})
			]
		});
		await vi.waitFor(() => expect(railTitles().length).toBe(1));

		await vi.waitFor(() => expect(document.querySelector('[data-testid="step-ref-name"]')).not.toBeNull());
		expect(panelInputs(), 'a referenced action has no edit fields').toHaveLength(0);
		expect(document.querySelector('[data-testid="action-state-toggle"]')).toBeNull();

		const rowState = [
			...document.querySelectorAll<HTMLButtonElement>('[data-testid="step-row-state"] button')
		];
		expect(rowState.length, 'the row still states the action state').toBeGreaterThan(0);
		expect(
			rowState.every((b) => b.disabled),
			'a library action cannot be re-stated from a set'
		).toBe(true);
		rowState.forEach((b) => b.click());
		await new Promise((r) => setTimeout(r, 50));

		const link = document.querySelector<HTMLAnchorElement>('[data-testid="step-ref-link"]');
		expect(link?.getAttribute('href')).toContain(`/actions/${PKG_ID}`);

		await vi.waitFor(() => expect(shell.pill.context).not.toBeNull());
		commitContext();

		await vi.waitFor(() => expect(api.renameActionSet).toHaveBeenCalledTimes(0));
		expect(api.updateActionParams, 'the shared action is untouched').not.toHaveBeenCalled();
		expect(api.renameAction).not.toHaveBeenCalled();
		expect(api.updateActionDescription).not.toHaveBeenCalled();
	});
});

describe('the palette leads with what already exists', () => {
	it('offers the library on the first tab, and authoring behind the second', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		const tabs = [...document.querySelectorAll<HTMLButtonElement>('[data-palette-tab]')];
		expect(tabs.map((t) => t.dataset.paletteTab)).toEqual(['existing', 'new']);
		expect(tabs[0].getAttribute('aria-selected')).toBe('true');

		const offered = [...document.querySelectorAll<HTMLElement>('[data-palette-entry]')].map(
			(e) => e.dataset.paletteEntry
		);
		expect(offered).toContain(SPARE_ID);
		expect(offered, 'a current member is not offered twice').not.toContain(PKG_ID);

		document.querySelector<HTMLButtonElement>(`[data-palette-entry="${SPARE_ID}"]`)!.click();
		await vi.waitFor(() => expect(railTitles().length).toBe(3));
		expect(api.createAction).not.toHaveBeenCalled();

		await paletteNewTab();
		expect(document.querySelector('[data-palette-entry="SERVICE"]')).not.toBeNull();
	});
});

describe('stash and restore', () => {
	it('parks the draft with its step position and lands back on the same step', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		await paletteNewTab();
		const svcEntry = document.querySelector<HTMLButtonElement>('[data-palette-entry="SERVICE"]');
		if (!svcEntry) throw new Error('the palette never offered the SERVICE action type');
		svcEntry.click();
		await vi.waitFor(() => expect(railTitles().length).toBe(3));
		await vi.waitFor(() => expect(panelInputs().length).toBeGreaterThan(0));
		type(panelField('unitName'), 'ssh');

		await vi.waitFor(() => expect(shell.pill.context).not.toBeNull());
		expect(shell.pill.context?.stashSubtitle).toBe(
			m.action_set_detail_builder_stash_subtitle({ step: 3 })
		);

		const draftId = stashContext();
		expect(draftId).not.toBeNull();

		expect(shell.pill.context).toBeNull();
		expect(shell.drafts.map((d) => d.id)).toContain(draftId);

		await new Promise((r) => setTimeout(r, 50));
		expect(shell.pill.context).toBeNull();

		expect(restoreDraft(draftId!)).toBeNull();
		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`action-set:${SET_ID}`));

		expect(shell.pill.context?.stashSubtitle).toBe(
			m.action_set_detail_builder_stash_subtitle({ step: 3 })
		);
		expect(railTitles()).toHaveLength(3);
		expect(
			[...document.querySelectorAll<HTMLInputElement>('input')].some((i) => i.value === 'ssh')
		).toBe(true);
	});
});

describe('reorder', () => {
	it('sends reorderActionInSet for exactly the steps whose index moved', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		const up = document.querySelectorAll<HTMLButtonElement>(
			`button[aria-label="${m.action_set_detail_builder_move_up()}"]`
		)[1];
		up.click();

		await vi.waitFor(() => expect(railTitles()).toEqual(['Enable sshd', 'Install openssh-server']));
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.reorderActionInSet).toHaveBeenCalledTimes(2));
		expect(api.reorderActionInSet.mock.calls).toEqual([
			[SET_ID, SVC_ID, 0],
			[SET_ID, PKG_ID, 1]
		]);

		expect(api.removeActionFromSet).not.toHaveBeenCalled();
		expect(api.createAction).not.toHaveBeenCalled();
		expect(api.renameActionSet).not.toHaveBeenCalled();
	});
});

describe('palette insert', () => {
	it('appends a step of the inserted action type and creates it on commit', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		await paletteNewTab();
		const entry = document.querySelector<HTMLButtonElement>('[data-palette-entry="SERVICE"]');
		if (!entry) throw new Error('the palette never offered the SERVICE action type');
		entry.click();

		await vi.waitFor(() => expect(railTitles().length).toBe(3));

		expect(
			document.querySelector('[data-step-key][aria-current="true"]')?.textContent
		).toContain(m.actions_type_systemd());

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));

		await vi.waitFor(() => expect(panelField('unitName').value).toBe(''));
		type(panelField('unitName'), 'nginx');

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.createAction).toHaveBeenCalledTimes(1));
		expect(api.createAction.mock.calls[0][0]).toMatchObject({
			type: ActionType.SERVICE,
			params: { case: 'service' }
		});

		await vi.waitFor(() => expect(api.addActionToSet).toHaveBeenCalledTimes(1));
		expect(api.addActionToSet.mock.calls[0][2]).toBe(2);
		expect(api.reorderActionInSet).not.toHaveBeenCalled();
	});
});
