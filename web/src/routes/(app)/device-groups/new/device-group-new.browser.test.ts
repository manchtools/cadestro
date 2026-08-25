

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createDeviceGroup: vi.fn(),
	deleteDeviceGroup: vi.fn(),
	search: vi.fn(),
	validateDynamicQuery: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/device-groups/new') }));

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

const ROUTE = '/device-groups/new';
const QUERY = 'true';
const GROUP_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/device-groups/new');
	api.createDeviceGroup.mockResolvedValue({ id: { value: GROUP_ID }, name: 'Berlin laptops' });
	api.search.mockResolvedValue({ results: [], totalCount: 0n, nextPageToken: '' });
	api.validateDynamicQuery.mockResolvedValue({ valid: true, count: 0, error: '' });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);
const area = (id: string) => document.querySelector<HTMLTextAreaElement>(`#${id}`);

async function queryInput() {
	return await vi.waitFor(() => {
		const input = area('query-editor-text');
		expect(input).toBeTruthy();
		return input!;
	});
}

function type(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillGroup(name: string, description: string) {
	await vi.waitFor(() => expect(field('group-name')).toBeTruthy());
	type(field('group-name')!, name);
	type(area('group-description')!, description);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/device-groups/new — the commit is the pill\'s', () => {
	it('declares a route, which is what earns the Stash button', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin laptops', 'Floor 3');

		expect(shell.pill.context?.route).toBe(ROUTE);

		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
	});

	it('creates the group with the exact arguments the dialog sent', async () => {
		render(NewGroupPage);
		await fillGroup('  Berlin laptops  ', '  Floor 3  ');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createDeviceGroup).toHaveBeenCalledTimes(1));
		expect(api.createDeviceGroup.mock.calls[0]).toEqual([
			'Berlin laptops',
			'Floor 3',
			undefined
		]);
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/device-groups/${GROUP_ID}`)
		);
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin laptops', '');

		type(field('group-name')!, '   ');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createDeviceGroup).not.toHaveBeenCalled();
	});

	it('accepts true as an explicit CEL query', async () => {
		render(NewGroupPage);
		await fillGroup('Every device', '');

		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		type(await queryInput(), QUERY);
		await vi.waitFor(() => expect(api.validateDynamicQuery).toHaveBeenCalledWith(QUERY), { timeout: 3000 });
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createDeviceGroup).toHaveBeenCalledTimes(1));
		expect(api.createDeviceGroup.mock.calls[0]).toEqual(['Every device', '', QUERY]);
	});

	it('shows the live match count beside the builder while editing a rule', async () => {
		api.validateDynamicQuery.mockResolvedValue({
			valid: true,
			error: '',
			matchingDeviceCount: 5
		});
		render(NewGroupPage);
		await fillGroup('Berlin laptops', '');

		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		type(await queryInput(), 'device.hostname == "web-prod-01"');

		await vi.waitFor(() => expect(api.validateDynamicQuery).toHaveBeenCalledWith('device.hostname == "web-prod-01"'), { timeout: 3000 });
		await vi.waitFor(
			() =>
				expect(
					document.querySelector('[data-testid="query-status"]')?.textContent
				).toContain(m.query_match_count_devices({ count: 5 })),
			{ timeout: 3000 }
		);
	});

	it('still refuses the commit while a dynamic rule is partially filled', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin laptops', '');

		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		type(await queryInput(), 'device.hostname ==');
		api.validateDynamicQuery.mockResolvedValue({ valid: false, error: 'invalid CEL', matchingDeviceCount: 0 });
		await vi.waitFor(() => expect(api.validateDynamicQuery).toHaveBeenCalledWith('device.hostname =='), { timeout: 3000 });

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false), { timeout: 3000 });
		expect(shell.pill.context?.subtext).toBe(m.device_groups_query_fix());
		expect(commitContext()).toBe(false);
		expect(api.createDeviceGroup).not.toHaveBeenCalled();
	});
});

describe('/device-groups/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer that still commits', async () => {
		const first = await render(NewGroupPage);
		await fillGroup('Berlin laptops', 'Floor 3');

		expect(stashContext()).toBe('draft:device-group:create');
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
		render(NewGroupPage);

		await vi.waitFor(() => expect(field('group-name')?.value).toBe('Berlin laptops'));
		expect(area('group-description')?.value).toBe('Floor 3');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createDeviceGroup).toHaveBeenCalledTimes(1));
		expect(api.createDeviceGroup.mock.calls[0][0]).toBe('Berlin laptops');
	});

	it('parks the membership mode too, not only the two text fields', async () => {
		render(NewGroupPage);
		await fillGroup('Berlin laptops', '');
		document.querySelector<HTMLButtonElement>('#group-dynamic')!.click();
		type(await queryInput(), QUERY);
		await vi.waitFor(() => expect(api.validateDynamicQuery).toHaveBeenCalledWith(QUERY), { timeout: 3000 });

		stashContext();
		expect(shell.drafts[0].payload).toMatchObject({ name: 'Berlin laptops', isDynamic: true });
	});
});

describe('/device-groups — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {
		nav.url = new URL('https://control.test/device-groups');
		render(GroupsPage);

		const create = await vi.waitFor(() => {
			const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
				(b) => b.textContent?.trim() === m.device_groups_create()
			);
			expect(button).toBeTruthy();
			return button!;
		});
		create.click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/device-groups/new'));
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
	});
});
