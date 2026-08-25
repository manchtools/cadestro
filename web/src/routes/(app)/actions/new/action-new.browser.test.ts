

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { ActionType } from '$contract/cadestro/v1/actions_pb';
import { ACTION_REGISTRY, FORM_KEYS } from '$lib/components/actions/registry';
import { getActionTypeInfoByValue } from '$lib/components/actions/action-type';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createAction: vi.fn(),
	listActions: vi.fn(),
	listUsers: vi.fn()
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
vi.mock('$app/state', () => ({ page: { url: new URL('https://control.test/actions/new'), params: {} } }));

import { goto } from '$app/navigation';
import NewActionPage from './+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';
import { TILE_VALUES, tileFormKeys } from './type-tiles';

const ROUTE = '/actions/new';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	api.createAction.mockResolvedValue({ id: { value: '01JQZZBH4V0W6Y9Z3A5B8C7D2E' }});
	api.listActions.mockResolvedValue({ actions: [], nextPageToken: '' });
	api.listUsers.mockResolvedValue({ users: [], nextPageToken: '' });
});

const tiles = () => document.querySelectorAll<HTMLElement>('[data-testid="action-type-tile"]');
const tile = (value: string) =>
	document.querySelector<HTMLButtonElement>(`[data-type-value="${value}"]`);
const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillPackageDraft(name: string, pkg: string) {
	tile('PACKAGE')!.click();
	await vi.waitFor(() => expect(field('action-name')).toBeTruthy());
	type(field('action-name')!, name);
	await vi.waitFor(() => expect(field('packageName')).toBeTruthy());
	type(field('packageName')!, pkg);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('the type wall is the registry', () => {
	it('renders one tile per creatable action type, covering ACTION_REGISTRY exactly', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();

		expect(FORM_KEYS.length).toBeGreaterThan(0);
		expect(TILE_VALUES.length).toBeGreaterThanOrEqual(FORM_KEYS.length);

		expect([...tileFormKeys()].sort()).toEqual([...FORM_KEYS].sort());
		expect(tileFormKeys().every((k) => k in ACTION_REGISTRY)).toBe(true);

		expect(tiles()).toHaveLength(TILE_VALUES.length);
		for (const value of TILE_VALUES) expect(tile(value)).toBeTruthy();
	});

	it('names and describes each tile from the shared i18n type info', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();

		const info = getActionTypeInfoByValue('PACKAGE');
		expect(tile('PACKAGE')!.textContent).toContain(info.label);
		expect(tile('PACKAGE')!.textContent).toContain(info.description);
	});

	it('filters the wall live, and says so when nothing matches', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();

		const filter = document.querySelector<HTMLInputElement>('[data-testid="action-type-filter"]')!;
		type(filter, 'flatpak');
		await vi.waitFor(() => expect(tiles()).toHaveLength(1));
		expect(tile('FLATPAK')).toBeTruthy();

		type(filter, 'zzzz-no-such-type');
		await vi.waitFor(() => expect(tiles()).toHaveLength(0));
		await expect.element(browser.getByText(m.common_no_results_search())).toBeVisible();
	});
});

describe('choose → configure → commit', () => {
	it('carries no Save button of its own — the commit is the pill\'s', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();
		await fillPackageDraft('Install nginx', 'nginx');

		expect(browser.getByRole('button', { name: m.common_save() }).elements()).toHaveLength(0);
		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
	});

	it('blocks the commit at the STORE until the type\'s own schema is satisfied', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();

		tile('PACKAGE')!.click();
		await vi.waitFor(() => expect(field('action-name')).toBeTruthy());
		type(field('action-name')!, 'Install nginx');

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createAction).not.toHaveBeenCalled();
	});

	it('creates the action with the exact typed params the registry produced', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();
		await fillPackageDraft('Install nginx', 'nginx');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createAction).toHaveBeenCalledTimes(1));

		const request = api.createAction.mock.calls[0][0];
		expect(request.name).toBe('Install nginx');
		expect(request.type).toBe(ActionType.PACKAGE);
		expect(request.desiredState).toBe(0);
		expect(request.params.case).toBe('package');
		expect(request.params.value.name).toBe('nginx');

		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/actions/01JQZZBH4V0W6Y9Z3A5B8C7D2E')
		);
	});

	it('sends ABSENT when the operator flips the state toggle a type actually offers', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();
		await fillPackageDraft('Remove nginx', 'nginx');

		document.querySelector<HTMLButtonElement>('[data-state-value="1"]')!.click();
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));
		expect(commitContext()).toBe(true);

		await vi.waitFor(() => expect(api.createAction).toHaveBeenCalledTimes(1));
		expect(api.createAction.mock.calls[0][0].desiredState).toBe(1);
	});

	it('offers no ABSENT at all for a type that does not model it', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();

		expect(ACTION_REGISTRY.UPDATE.supportsAbsent).toBe(false);
		tile('UPDATE')!.click();
		await expect.element(browser.getByTestId('action-state-toggle')).toBeVisible();

		expect(document.querySelectorAll('[data-state-value]')).toHaveLength(1);
		expect(document.querySelector('[data-state-value="1"]')).toBeNull();
	});

	it('keeps the buffer when the operator steps back to the wall and re-picks the same type', async () => {
		render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();
		await fillPackageDraft('Install nginx', 'nginx');

		document.querySelector<HTMLButtonElement>('[data-testid="action-configure-back"]')!.click();
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();

		tile('PACKAGE')!.click();
		await vi.waitFor(() => expect(field('action-name')?.value).toBe('Install nginx'));
		expect(field('packageName')?.value).toBe('nginx');
	});
});

describe('the third exit — stash, walk away, restore', () => {

	it('navigates home and rebuilds a form that still commits', async () => {
		const first = await render(NewActionPage);
		await expect.element(browser.getByTestId('action-type-chooser')).toBeVisible();
		await fillPackageDraft('Install nginx', 'nginx');

		const draftId = stashContext();
		expect(draftId).toBe('draft:action:create');
		expect(shell.drafts[0].route).toBe(ROUTE);
		expect(shell.drafts[0].subtitle).toBe(
			m.actions_new_stash_subtitle({ type: getActionTypeInfoByValue('PACKAGE').label })
		);

		await new Promise((r) => setTimeout(r, 50));
		expect(pillMode()).toBe('nav');

		await first.unmount();
		setShellPath('/devices');
		const rail = await render(StageRail);
		await browser.getByTestId('stage-draft').click();

		expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(ROUTE);

		expect(pillMode()).toBe('nav');
		expect(shell.drafts).toHaveLength(0);
		await rail.unmount();

		setShellPath(ROUTE);
		render(NewActionPage);

		await vi.waitFor(() => expect(field('action-name')?.value).toBe('Install nginx'));
		expect(field('packageName')?.value).toBe('nginx');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createAction).toHaveBeenCalledTimes(1));
		expect(api.createAction.mock.calls[0][0].params.value.name).toBe('nginx');
	});
});
