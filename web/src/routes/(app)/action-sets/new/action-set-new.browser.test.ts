

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import { ManagedActionSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createActionSet: vi.fn(),
	addActionToSet: vi.fn(),
	listActions: vi.fn(),
	listActionSets: vi.fn(),
	getActionSet: vi.fn(),
	deleteActionSet: vi.fn(),
	listUsers: vi.fn(),
	search: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/action-sets/new') }));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: { value: '01JQZZ0000000000000000000A' }}, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn(async () => []),
		persistDraft: () => {},
		useDraft: <T>(_type: string, _id: string, initial: T) => {
			let data = initial;
			return {
				get data() {
					return data;
				},
				set data(next: T) {
					data = next;
				},
				update() {},
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
vi.mock('$app/state', () => ({
	page: {
		get url() {
			return nav.url;
		},
		params: {}
	}
}));

import { goto } from '$app/navigation';
import NewSetPage from './+page.svelte';
import SetsPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/action-sets/new';
const SET_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const ACTION_A = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const ACTION_B = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';

const catalogue = [
	create(ManagedActionSchema, { id: { value: ACTION_A }, name: 'Install nginx', type: ActionType.PACKAGE }),
	create(ManagedActionSchema, { id: { value: ACTION_B }, name: 'Patch kernel', type: ActionType.UPDATE })
];

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/action-sets/new');
	api.createActionSet.mockResolvedValue({ id: { value: SET_ID }, name: 'Laptop baseline' });
	api.addActionToSet.mockResolvedValue({});
	api.listActions.mockResolvedValue({ actions: catalogue, nextPageToken: '' });
	api.listUsers.mockResolvedValue({ users: [], nextPageToken: '' });
	api.search.mockResolvedValue({ results: [], totalCount: 0n, nextPageToken: '' });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);
const area = (id: string) => document.querySelector<HTMLTextAreaElement>(`#${id}`);
const actionRow = (id: string) =>
	document.querySelector<HTMLButtonElement>(`[data-testid="set-action-row"][data-action-id="${id}"]`);

function type(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillSet(name: string, description: string) {
	await vi.waitFor(() => expect(field('set-name')).toBeTruthy());
	type(field('set-name')!, name);
	type(area('set-description')!, description);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/action-sets/new — one page, no wizard footer', () => {
	it('declares a route and shows the picker without a "Next" step', async () => {
		render(NewSetPage);
		await fillSet('Laptop baseline', 'Everything a laptop needs');
		await vi.waitFor(() => expect(actionRow(ACTION_A)).toBeTruthy(), { timeout: 3000 });

		expect(shell.pill.context?.route).toBe(ROUTE);
		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
		expect(document.querySelector('[data-testid="action-set-pick"]')).not.toBeNull();
	});

	it('creates the set and adds every picked action in selection order', async () => {
		render(NewSetPage);
		await fillSet('  Laptop baseline  ', '  Everything a laptop needs  ');
		await vi.waitFor(() => expect(actionRow(ACTION_B)).toBeTruthy(), { timeout: 3000 });

		actionRow(ACTION_B)!.click();
		actionRow(ACTION_A)!.click();
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="set-selected-count"]')?.textContent?.trim()).toBe(
				m.picker_selected({ count: '2' })
			)
		);

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createActionSet).toHaveBeenCalledTimes(1));
		const request = api.createActionSet.mock.calls[0][0];
		expect(request.name).toBe('Laptop baseline');
		expect(request.description).toBe('Everything a laptop needs');
		expect(request.schedule.intervalHours).toBe(8);

		await vi.waitFor(() => expect(api.addActionToSet).toHaveBeenCalledTimes(2));
		expect(api.addActionToSet.mock.calls[0]).toEqual([SET_ID, ACTION_B, 0]);
		expect(api.addActionToSet.mock.calls[1]).toEqual([SET_ID, ACTION_A, 1]);
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/action-sets/${SET_ID}`)
		);
	});

	it('creates an empty set when nothing is picked, exactly as "Skip" did', async () => {
		render(NewSetPage);
		await fillSet('Laptop baseline', '');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createActionSet).toHaveBeenCalledTimes(1));
		expect(api.addActionToSet).not.toHaveBeenCalled();
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewSetPage);
		await fillSet('Laptop baseline', '');

		type(field('set-name')!, '   ');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createActionSet).not.toHaveBeenCalled();
	});
});

describe('/action-sets/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer — selection included — that still commits', async () => {
		const first = await render(NewSetPage);
		await fillSet('Laptop baseline', 'Everything a laptop needs');
		await vi.waitFor(() => expect(actionRow(ACTION_A)).toBeTruthy(), { timeout: 3000 });
		actionRow(ACTION_A)!.click();
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="set-selected-count"]')?.textContent?.trim()).toBe(
				m.picker_selected({ count: '1' })
			)
		);

		expect(stashContext()).toBe('draft:action-set:create');
		expect(shell.drafts[0].route).toBe(ROUTE);
		await new Promise((r) => setTimeout(r, 50));
		expect(pillMode()).toBe('nav');

		await first.unmount();
		setShellPath('/devices');
		const rail = await render(StageRail);
		(document.querySelector('[data-testid="stage-draft"]') as HTMLElement).click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(ROUTE));
		expect(pillMode()).toBe('nav');

		expect(shell.drafts).toHaveLength(0);
		await rail.unmount();

		setShellPath(ROUTE);
		render(NewSetPage);

		await vi.waitFor(() => expect(field('set-name')?.value).toBe('Laptop baseline'));
		expect(area('set-description')?.value).toBe('Everything a laptop needs');

		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="set-selected-count"]')?.textContent?.trim()).toBe(
				m.picker_selected({ count: '1' })
			)
		);
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.addActionToSet).toHaveBeenCalledTimes(1));
		expect(api.addActionToSet.mock.calls[0]).toEqual([SET_ID, ACTION_A, 0]);
	});
});

describe('/action-sets — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {

		nav.url = new URL('https://control.test/action-sets?zoom=list');

		api.listActions.mockResolvedValue({ actions: [], nextPageToken: '' });
		const list = await render(SetsPage);

		await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });

		const createButton = await vi.waitFor(() => {
			const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
				(b) => b.textContent?.trim() === m.action_sets_create()
			);
			expect(button).toBeTruthy();
			return button!;
		});
		createButton.click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/action-sets/new'));
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
		await list.unmount();
	});
});
