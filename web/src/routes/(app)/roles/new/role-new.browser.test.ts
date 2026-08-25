

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createRole: vi.fn(),
	listRoles: vi.fn(),
	deleteRole: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/roles/new') }));

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
import NewRolePage from './+page.svelte';
import RolesPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/roles/new';
const ROLE_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/roles/new');
	api.createRole.mockResolvedValue({ id: ROLE_ID, name: 'Auditor' });
	api.listRoles.mockResolvedValue({ roles: [] });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillRole(name: string, description: string) {
	await vi.waitFor(() => expect(field('role-name')).toBeTruthy());
	type(field('role-name')!, name);
	type(field('role-description')!, description);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/roles/new — the commit is the pill\'s', () => {
	it('declares a route, which is what earns the Stash button', async () => {
		render(NewRolePage);
		await fillRole('Auditor', 'Read-only reviewer');

		expect(shell.pill.context?.route).toBe(ROUTE);

		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
	});

	it('creates the role with the exact arguments the dialog sent', async () => {
		render(NewRolePage);
		await fillRole('  Auditor  ', '  Read-only reviewer  ');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createRole).toHaveBeenCalledTimes(1));
		expect(api.createRole.mock.calls[0]).toEqual(['Auditor', 'Read-only reviewer', []]);
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/roles/${ROLE_ID}`)
		);
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewRolePage);
		await fillRole('Auditor', '');

		type(field('role-name')!, '   ');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(shell.pill.context?.subtext).toBe(m.validation_name_required());
		expect(commitContext()).toBe(false);
		expect(api.createRole).not.toHaveBeenCalled();
	});
});

describe('/roles/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer that still commits', async () => {
		const first = await render(NewRolePage);
		await fillRole('Auditor', 'Read-only reviewer');

		expect(stashContext()).toBe('draft:role:create');
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
		render(NewRolePage);

		await vi.waitFor(() => expect(field('role-name')?.value).toBe('Auditor'));
		expect(field('role-description')?.value).toBe('Read-only reviewer');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createRole).toHaveBeenCalledTimes(1));
		expect(api.createRole.mock.calls[0]).toEqual(['Auditor', 'Read-only reviewer', []]);
	});
});

describe('/roles — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {
		nav.url = new URL('https://control.test/roles');
		render(RolesPage);
		await vi.waitFor(() => expect(api.listRoles).toHaveBeenCalled(), { timeout: 3000 });

		const create = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
			(b) => b.textContent?.trim() === m.roles_create()
		);
		expect(create).toBeTruthy();
		create!.click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/roles/new'));
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
	});
});
