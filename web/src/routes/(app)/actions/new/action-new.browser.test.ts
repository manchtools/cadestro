// Behaviour contract for the rebuilt /actions/new.
//
// What these pin is the difference between "a nicer type list" and a surface
// that is actually load-bearing:
//   - the type wall is DERIVED from ACTION_REGISTRY, so a newly registered
//     adapter is creatable the moment it is registered (and no tile can exist
//     for an adapter that isn't there);
//   - choosing a type and committing reaches createAction with the exact typed
//     params, through the registry's own formToProto — not a re-typed copy;
//   - and the operator's reported round trip: stash, walk away, restore. That
//     one must navigate home and rebuild a form that can still commit, because
//     re-entering the parked context handed back dead closures.
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import { ACTION_REGISTRY, FORM_KEYS } from '$lib/components/actions/registry';
import { getActionTypeInfoByValue } from '$lib/components/actions/action-type';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createAction: vi.fn(),
	listActions: vi.fn(),
	listUsers: vi.fn()
}));

// Only the client and the IndexedDB-backed draft hook are faked; the generated
// protobuf re-exports stay real, so the registry, the zod schemas and the
// ActionType constants under test are the production ones.
//
// The draft hook is deliberately MEMORYLESS across mounts: a remount gets an
// empty autosave, so the cross-route test can only pass if the stage card's own
// payload rebuilt the form.
vi.mock('$lib/sdk', async () => {
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
	const common = await import('$sdk/powermanage/v1/common_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		fetchAllPages: vi.fn(async () => []),
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
	api.createAction.mockResolvedValue({ id: '01JQZZBH4V0W6Y9Z3A5B8C7D2E' });
	api.listActions.mockResolvedValue({ actions: [], nextPageToken: '' });
	api.listUsers.mockResolvedValue({ users: [], nextPageToken: '' });
});

const tiles = () => document.querySelectorAll<HTMLElement>('[data-testid="action-type-tile"]');
const tile = (value: string) =>
	document.querySelector<HTMLButtonElement>(`[data-type-value="${value}"]`);
const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

/** Type into a real input the way the browser does, so Svelte's binding sees it. */
function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

/** Choose a type, name the action and give the package form a resolvable name. */
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

		// Matches-zero guard: an empty registry (or an empty wall) must fail loudly
		// rather than make the coverage assertions below vacuously true.
		expect(FORM_KEYS.length).toBeGreaterThan(0);
		expect(TILE_VALUES.length).toBeGreaterThanOrEqual(FORM_KEYS.length);

		// Self-discovering, both directions: every registry adapter is reachable
		// from the wall, and every tile resolves to a real adapter.
		expect([...tileFormKeys()].sort()).toEqual([...FORM_KEYS].sort());
		expect(tileFormKeys().every((k) => k in ACTION_REGISTRY)).toBe(true);

		// …and the DOM shows exactly the derived set, not a hand-written subset.
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

		// A named action with no package name satisfies the basic schema and fails
		// the PACKAGE schema — the guard is the store's, so ⌘S is closed too.
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
		// `$lib/navigation` forwards an options argument, so assert on the target.
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

		expect(ACTION_REGISTRY.UPDATE.supportsAbsent).toBe(false); // fixture guard
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
	// The operator's exact reproduction. Before the store contract changed, the
	// restore re-entered the parked ContextState and the pill came back pointing
	// at an unmounted page: "I can't see or do anything."
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
		// Parked means parked: the surface must not re-adopt its own card.
		await new Promise((r) => setTimeout(r, 50));
		expect(pillMode()).toBe('nav');

		// The operator navigates away — /actions/new unmounts and its component
		// state is gone — then clicks the card from wherever they ended up.
		await first.unmount();
		setShellPath('/devices');
		const rail = await render(StageRail);
		await browser.getByTestId('stage-draft').click();

		expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(ROUTE);
		// No dead context: the pill stays free. The card pops on the click and the
		// buffer is staged for the surface to claim on mount.
		expect(pillMode()).toBe('nav');
		expect(shell.drafts).toHaveLength(0);
		await rail.unmount();

		// …the navigation lands, the surface mounts and claims its own draft.
		setShellPath(ROUTE);
		render(NewActionPage);

		await vi.waitFor(() => expect(field('action-name')?.value).toBe('Install nginx'));
		expect(field('packageName')?.value).toBe('nginx');
		expect(shell.drafts).toHaveLength(0);

		// And the restored pill is LIVE — this is what the bug destroyed.
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createAction).toHaveBeenCalledTimes(1));
		expect(api.createAction.mock.calls[0][0].params.value.name).toBe('nginx');
	});
});
