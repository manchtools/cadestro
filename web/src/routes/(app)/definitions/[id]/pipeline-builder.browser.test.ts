// The definition builder is the same rail one level up, but a DIFFERENT commit
// sequence — it must reach the definition RPCs, never the action-set ones.
//
// Its pill is also the definition's ACTION BAR: held from mount, clean, carrying
// the actions the detail page handed down.
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import {
	ActionSetSchema,
	DefinitionSchema,
	type ActionSet,
	type Definition
} from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const DEF_ID = '01JQZZ8E1R7S3V6W0X2Y5Z4A9B';
const SET_A = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';
const SET_B = '01JQZZAG3T9V5X8Y2Z4A7B6C1D';
const SET_C = '01JQZZBH4V0W6Y9Z3A5B8C7D2E';

const api = vi.hoisted(() => ({
	addActionSetToDefinition: vi.fn(),
	removeActionSetFromDefinition: vi.fn(),
	reorderActionSetInDefinition: vi.fn(),
	renameDefinition: vi.fn(),
	updateDefinitionDescription: vi.fn(),
	getActionSet: vi.fn(),
	// The action-set builder's RPCs must never be reached from here.
	reorderActionInSet: vi.fn(),
	addActionToSet: vi.fn()
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
vi.mock('$app/state', () => ({ page: { url: new URL('https://control.test/definitions') } }));

import DefinitionBuilder from '$lib/components/actions/pipeline/definition-builder.svelte';
import { shell, resetShell, commitContext, runPillAction } from '$lib/shell/shell.svelte';

const definition: Definition = create(DefinitionSchema, {
	id: { value: DEF_ID },
	name: 'Workstation Setup',
	description: 'baseline'
});

function actionSet(id: string, name: string, memberCount: number): ActionSet {
	return create(ActionSetSchema, { id: { value: id }, name, memberCount });
}

/** Deliberately out of order — the rail must sort by sortOrder. */
function members() {
	return [
		{ actionSetId: SET_B, sortOrder: 1, actionSetName: 'Harden SSH baseline' },
		{ actionSetId: SET_A, sortOrder: 0, actionSetName: 'Base System Setup' }
	];
}

/** The detail page hands the DEFINITION's own actions down to the builder; a
 *  recorder stands in for the page's confirm dialog. */
let deleteRuns = 0;

function mount() {
	return render(DefinitionBuilder, {
		props: {
			defId: DEF_ID,
			definition,
			members: members(),
			library: [
				actionSet(SET_A, 'Base System Setup', 2),
				actionSet(SET_B, 'Harden SSH baseline', 4),
				actionSet(SET_C, 'Baseline inventory', 1)
			],
			entityActions: [
				{
					id: 'delete',
					label: m.definitions_delete_definition(),
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

beforeEach(() => {
	resetShell();
	vi.clearAllMocks();
	deleteRuns = 0;
	api.getActionSet.mockResolvedValue({ members: [] });
});

describe('the pill is the definition’s action bar', () => {
	it('holds the definition from mount, clean, carrying the page’s actions', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		await vi.waitFor(() => expect(shell.pill.context?.id).toBe(`definition:${DEF_ID}`));
		// Nothing edited yet: nothing to save, and ⌘S is closed in the store.
		expect(shell.pill.context?.dirty).toBe(false);
		expect(commitContext()).toBe(false);
		expect(api.renameDefinition).not.toHaveBeenCalled();

		expect(shell.pill.context?.extraActions?.map((a) => a.id)).toEqual(['delete']);
		runPillAction('delete');
		expect(deleteRuns).toBe(1);
		expect(api.removeActionSetFromDefinition).not.toHaveBeenCalled();
	});
});

describe('definition pipeline', () => {
	it('renders member sets in sortOrder', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));
		expect(railTitles()).toEqual(['Base System Setup', 'Harden SSH baseline']);
	});

	it('offers only non-member sets in the Movement C picker', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		const offered = [...document.querySelectorAll('[data-palette-entry]')].map((el) =>
			el.getAttribute('data-palette-entry')
		);
		expect(offered).toEqual([SET_C]);
	});

	it('appends a picked set and reorders only what moved, via the definition RPCs', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		// Append "Baseline inventory" from the palette …
		document.querySelector<HTMLButtonElement>(`[data-palette-entry="${SET_C}"]`)!.click();
		await vi.waitFor(() => expect(railTitles().length).toBe(3));

		// … then move it to the front, which shifts both existing members.
		const upButtons = document.querySelectorAll<HTMLButtonElement>(
			`button[aria-label="${m.action_set_detail_builder_move_up()}"]`
		);
		upButtons[2].click();
		upButtons[1].click();
		await vi.waitFor(() =>
			expect(railTitles()).toEqual([
				'Baseline inventory',
				'Base System Setup',
				'Harden SSH baseline'
			])
		);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.addActionSetToDefinition).toHaveBeenCalledTimes(1));
		expect(api.addActionSetToDefinition).toHaveBeenCalledWith(DEF_ID, SET_C, 0);
		await vi.waitFor(() => expect(api.reorderActionSetInDefinition).toHaveBeenCalledTimes(2));
		expect(api.reorderActionSetInDefinition.mock.calls).toEqual([
			[DEF_ID, SET_A, 1],
			[DEF_ID, SET_B, 2]
		]);
		// A definition never writes through the action-set surface.
		expect(api.reorderActionInSet).not.toHaveBeenCalled();
		expect(api.addActionToSet).not.toHaveBeenCalled();
	});

	it('blocks the commit while the definition name is empty', async () => {
		mount();
		await vi.waitFor(() => expect(railTitles().length).toBe(2));

		const nameInput = document.querySelector<HTMLInputElement>('input#def-name')!;
		nameInput.value = '';
		nameInput.dispatchEvent(new Event('input', { bubbles: true }));

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(shell.pill.context?.subtextTone).toBe('warn');
		expect(api.renameDefinition).not.toHaveBeenCalled();
	});
});
