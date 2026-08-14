// Behaviour contract for /user-groups/new — the create flow that used to be a
// modal. This one carried a whole query builder inside a dialog, so it is the
// clearest case of the operator's report: real unfinished work with no way to
// park it. The tests pin the same RPC arguments, the store-level gate (including
// the unusable-rule refusal), the cross-route stash round trip, and a Create
// button that navigates instead of popping a dialog.
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createUserGroup: vi.fn(),
	deleteUserGroup: vi.fn(),
	search: vi.fn(),
	validateUserGroupQuery: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/user-groups/new') }));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: '01JQZZ0000000000000000000A' }, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',
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
import NewGroupPage from './+page.svelte';
import GroupsPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/user-groups/new';
const GROUP_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/user-groups/new');
	api.createUserGroup.mockResolvedValue({ id: GROUP_ID, name: 'Berlin staff' });
	api.search.mockResolvedValue({ results: [], totalCount: 0n, nextPageToken: '' });
	api.validateUserGroupQuery.mockResolvedValue({ valid: true, count: 0, error: '' });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillGroup(name: string, description: string) {
	await vi.waitFor(() => expect(field('group-name')).toBeTruthy());
	type(field('group-name')!, name);
	type(field('group-description')!, description);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/user-groups/new — the commit is the pill\'s', () => {
	it('declares a route, which is what earns the Stash button', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin staff', 'Everyone in Berlin');

		expect(shell.pill.context?.route).toBe(ROUTE);
		// One commit grammar app-wide: create surfaces say Create.
		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
	});

	it('creates the group with the exact arguments the dialog sent', async () => {
		render(NewGroupPage);
		await fillGroup('  Berlin staff  ', '  Everyone in Berlin  ');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createUserGroup).toHaveBeenCalledTimes(1));
		// A static group sends an EMPTY query, not the buffer's — the dialog's rule.
		expect(api.createUserGroup.mock.calls[0]).toEqual([
			'Berlin staff',
			'Everyone in Berlin',
			false,
			''
		]);
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/user-groups/${GROUP_ID}`)
		);
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin staff', '');

		type(field('group-name')!, '   ');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createUserGroup).not.toHaveBeenCalled();
	});

	// Same legal-empty rule as the device twin: '' is the always-true tree, the
	// page advertises "matches all users", and the seeded pristine chip must not
	// bar the only way to say it from builder mode.
	it('allows an untouched builder to create a match-all group with an empty query', async () => {
		render(NewGroupPage);
		await fillGroup('Everyone', '');

		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="query-chip"]')).toBeTruthy(),
			{ timeout: 3000 }
		);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(document.body.textContent).toContain(m.user_groups_empty_query_warning());

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createUserGroup).toHaveBeenCalledTimes(1));
		expect(api.createUserGroup.mock.calls[0]).toEqual(['Everyone', '', true, '']);
	});

	// The empty rule is the legal match-all rule; the server counts it like any
	// other, so the untouched builder shows the org-wide user count, not nothing.
	it('fetches and shows the match-all count for the untouched empty rule', async () => {
		api.validateUserGroupQuery.mockResolvedValue({
			valid: true,
			error: '',
			matchingUserCount: 12
		});
		render(NewGroupPage);
		await fillGroup('Everyone', '');

		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();

		await vi.waitFor(() => expect(api.validateUserGroupQuery).toHaveBeenCalledWith(''), {
			timeout: 3000
		});
		await vi.waitFor(
			() =>
				expect(document.querySelector('[data-testid="query-status"]')?.textContent).toContain(
					m.query_match_count_users({ count: 12 })
				),
			{ timeout: 3000 }
		);
	});

	it('still refuses the commit while a dynamic rule is partially filled', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin staff', '');

		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		const chip = await vi.waitFor(() => {
			const el = document.querySelector<HTMLElement>('[data-testid="query-chip"]');
			expect(el).toBeTruthy();
			return el!;
		});
		chip.click();
		await browser.getByLabelText(m.qb_placeholder_property()).click();
		await browser.getByRole('option', { name: m.qb_prop_user_email() }).click();

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false), { timeout: 3000 });
		expect(shell.pill.context?.subtext).toBe(m.user_groups_query_fix());
		expect(commitContext()).toBe(false);
		expect(api.createUserGroup).not.toHaveBeenCalled();
	});
});

describe('/user-groups/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer that still commits', async () => {
		const first = await render(NewGroupPage);
		await fillGroup('Berlin staff', 'Everyone in Berlin');

		expect(stashContext()).toBe('draft:user-group:create');
		expect(shell.drafts[0].route).toBe(ROUTE);
		await new Promise((r) => setTimeout(r, 50));
		expect(pillMode()).toBe('nav');

		await first.unmount();
		setShellPath('/devices');
		const rail = await render(StageRail);
		(document.querySelector('[data-testid="stage-draft"]') as HTMLElement).click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(ROUTE));
		expect(pillMode()).toBe('nav');
		// The card pops on the click; the buffer is staged for the remount.
		expect(shell.drafts).toHaveLength(0);
		await rail.unmount();

		setShellPath(ROUTE);
		render(NewGroupPage);

		await vi.waitFor(() => expect(field('group-name')?.value).toBe('Berlin staff'));
		expect(field('group-description')?.value).toBe('Everyone in Berlin');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createUserGroup).toHaveBeenCalledTimes(1));
		expect(api.createUserGroup.mock.calls[0][0]).toBe('Berlin staff');
	});

	it('parks the membership mode too, not only the two text fields', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin staff', '');
		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		// An untouched builder is now a legal (match-all) rule, so the pill stays
		// valid; wait on the builder itself, not on an invalid state.
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="query-chip"]')).toBeTruthy(),
			{ timeout: 3000 }
		);

		stashContext();
		expect(shell.drafts[0].payload).toMatchObject({ name: 'Berlin staff', isDynamic: true });
	});
});

describe('/user-groups — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {
		nav.url = new URL('https://control.test/user-groups');
		render(GroupsPage);

		const create = await vi.waitFor(() => {
			const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
				(b) => b.textContent?.trim() === m.user_groups_create()
			);
			expect(button).toBeTruthy();
			return button!;
		});
		create.click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/user-groups/new'));
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
	});
});
