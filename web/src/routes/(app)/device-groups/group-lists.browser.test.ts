

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema, type SearchResult } from '$contract/cadestro/v1/control_pb';
import { SearchScope, SortField, SortDirection } from '$contract/cadestro/v1/common_pb';
import { activePageSearch, resetPageSearch } from '$lib/shell/page-search.svelte';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/device-groups'),
	search: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return mocks.url;
		}
	}
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			search: mocks.search,
			deleteDeviceGroup: vi.fn(),
			deleteUserGroup: vi.fn(),
			createDeviceGroup: vi.fn(),
			createUserGroup: vi.fn(),
			validateDynamicQuery: vi.fn(),
			validateUserGroupQuery: vi.fn()
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages: vi.fn()
	};
});

import DeviceGroupsPage from './+page.svelte';
import UserGroupsPage from '../user-groups/+page.svelte';

function groupResult(id: string, name: string, fields: Record<string, string>): SearchResult {
	return create(SearchResultSchema, { id: { value: id }, name, description: '', fields });
}

function respond(results: SearchResult[]) {
	mocks.search.mockResolvedValue({ results, totalCount: results.length });
}

const STATIC_GROUP = groupResult('01HZDEVGRPSTATIC0000000000', 'Warehouse', {
	name: 'Warehouse',
	is_dynamic: 'false',
	member_count: '3',
	created_at: '1700000000'
});

const ARG = {
	query: 0,
	scope: 1,
	pageSize: 2,
	pageToken: 3,
	tags: 5,
	sortField: 6,
	sortDir: 7
} as const;

function lastCall() {
	return mocks.search.mock.calls.at(-1)!;
}

function rowKeys(): string[] {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="row-list-row"]')].map(
		(el) => el.getAttribute('data-row-key') ?? ''
	);
}

function rowLinks(): (string | null)[] {
	return [...document.querySelectorAll<HTMLAnchorElement>('[data-testid="row-list-link"]')].map(
		(a) => a.getAttribute('href')
	);
}

function clickSort(label: string) {
	const button = [
		...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
	].find((b) => b.textContent?.trim().startsWith(label));
	if (!button) throw new Error(`no sort control named ${label}`);
	button.click();
}

const DYNAMIC_GROUP = groupResult('01HZDEVGRPDYNAMIC000000000', 'Kiosks', {
	name: 'Kiosks',
	is_dynamic: 'true',
	member_count: '7',
	created_at: '1700000100'
});

beforeEach(() => {
	document.body.innerHTML = '';

	mocks.url = new URL('http://localhost/device-groups?zoom=list');
	mocks.search.mockReset();
	resetPageSearch();
	respond([STATIC_GROUP]);
});

describe('page-search registration does not loop the page (regression)', () => {
	it('renders rows with a live registration and no effect-depth error', async () => {
		const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
		try {
			respond([STATIC_GROUP, DYNAMIC_GROUP]);
			render(DeviceGroupsPage);

			await vi.waitFor(
				() =>
					expect(rowKeys()).toEqual([
						'01HZDEVGRPSTATIC0000000000',
						'01HZDEVGRPDYNAMIC000000000'
					]),
				{ timeout: 3000 }
			);

			expect(activePageSearch()?.scope).toBe(SearchScope.DEVICE_GROUPS);

			const depth = errors.mock.calls
				.flat()
				.map((a) => (a instanceof Error ? a.message : String(a)))
				.filter((text) => text.includes('effect_update_depth_exceeded'));
			expect(depth, 'Svelte reported a reactive update loop').toEqual([]);
		} finally {
			errors.mockRestore();
		}
	});
});

describe('device-groups list page', () => {
	it('queries the DEVICE_GROUPS scope sorted by NAME ascending by default', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());

		const call = lastCall();
		expect(call[ARG.scope]).toBe(SearchScope.DEVICE_GROUPS);
		expect(call[ARG.sortField]).toBe(SortField.NAME);
		expect(call[ARG.sortDir]).toBe(SortDirection.ASC);
		expect(call[ARG.tags]).toBeUndefined();
	});

	it('maps the member-count sort key to SortField.MEMBER_COUNT', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());
		const before = mocks.search.mock.calls.length;

		clickSort('Devices');

		await vi.waitFor(() => expect(mocks.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.sortField]).toBe(SortField.MEMBER_COUNT);
	});

	it('maps the created sort key to SortField.CREATED_AT, newest first', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());
		const before = mocks.search.mock.calls.length;

		clickSort('Created');

		await vi.waitFor(() => expect(mocks.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.sortField]).toBe(SortField.CREATED_AT);
		expect(lastCall()[ARG.sortDir]).toBe(SortDirection.DESC);
	});

	it('renders each group as a linked row in the row grammar — never a table', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(rowKeys()).toEqual([STATIC_GROUP.id?.value ?? '']));

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
		expect(rowLinks()).toEqual([`/device-groups/${STATIC_GROUP.id?.value ?? ''}`]);
	});

	it('round-trips a deep-linked sort / direction / page / page-size / type filter', async () => {
		mocks.url = new URL(
			'http://localhost/device-groups?sort=members&sortDir=desc&pageSize=10&page=3&type=dynamic&zoom=list'
		);
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());

		const call = lastCall();
		expect(call[ARG.pageSize]).toBe(10);
		expect(call[ARG.pageToken]).toBe('20');
		expect(call[ARG.sortField]).toBe(SortField.MEMBER_COUNT);
		expect(call[ARG.sortDir]).toBe(SortDirection.DESC);
		expect(call[ARG.tags]).toEqual({ is_dynamic: 'true' });
	});

	it('sends is_dynamic=false for the static type filter', async () => {
		mocks.url = new URL('http://localhost/device-groups?type=static&zoom=list');
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());

		expect(lastCall()[ARG.tags]).toEqual({ is_dynamic: 'false' });
	});

	it('sends no type tag when both membership modes are selected', async () => {
		mocks.url = new URL('http://localhost/device-groups?type=static,dynamic&zoom=list');
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());

		expect(lastCall()[ARG.tags]).toBeUndefined();
	});

	it('drives the type filter from the inline combobox', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());
		const before = mocks.search.mock.calls.length;

		const trigger = [...document.querySelectorAll('button')].find((b) =>
			b.textContent?.includes('All types')
		);
		expect(trigger, 'type filter combobox is rendered in the filter row').toBeTruthy();
		trigger!.click();

		const option = await vi.waitFor(() => {
			const el = [...document.querySelectorAll<HTMLElement>('[role="option"]')].find(
				(e) => e.textContent?.trim() === 'Dynamic'
			);
			if (!el) throw new Error('type filter did not offer a Dynamic option');
			return el;
		});
		option.click();

		await vi.waitFor(() => expect(mocks.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.tags]).toEqual({ is_dynamic: 'true' });
	});

	it('registers itself as the pill’s search scope and trims what reaches the server', async () => {
		resetPageSearch();
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());
		const before = mocks.search.mock.calls.length;

		const scoped = await vi.waitFor(() => {
			const reg = activePageSearch();
			if (!reg) throw new Error('the device-groups page registered no search scope');
			return reg;
		});
		expect(scoped.scope).toBe(SearchScope.DEVICE_GROUPS);

		scoped.setQuery('  ware  ');

		await vi.waitFor(() => expect(mocks.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.query]).toBe('ware');
		expect(scoped.query).toBe('  ware  ');
	});
});

describe('user-groups list page', () => {
	const SCIM_GROUP = groupResult('01HZUSRGRPSCIM00000000000', 'Directory', {
		name: 'Directory',
		is_dynamic: 'false',
		member_count: '5',
		is_scim_managed: 'true',
		created_at: '1700000000'
	});
	const LOCAL_GROUP = groupResult('01HZUSRGRPLOCAL0000000000', 'Operators', {
		name: 'Operators',
		is_dynamic: 'false',
		member_count: '2',
		is_scim_managed: 'false',
		created_at: '1700000000'
	});

	beforeEach(() => {
		mocks.url = new URL('http://localhost/user-groups?zoom=list');
		respond([SCIM_GROUP, LOCAL_GROUP]);
	});

	it('queries the USER_GROUPS scope sorted by NAME ascending by default', async () => {
		render(UserGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());

		const call = lastCall();
		expect(call[ARG.scope]).toBe(SearchScope.USER_GROUPS);
		expect(call[ARG.sortField]).toBe(SortField.NAME);
		expect(call[ARG.sortDir]).toBe(SortDirection.ASC);
	});

	it('maps the member sort key to SortField.MEMBER_COUNT', async () => {
		render(UserGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());
		const before = mocks.search.mock.calls.length;

		clickSort('Members');

		await vi.waitFor(() => expect(mocks.search.mock.calls.length).toBeGreaterThan(before));
		expect(lastCall()[ARG.sortField]).toBe(SortField.MEMBER_COUNT);
	});

	it('renders each group as a linked row in the row grammar — never a table', async () => {
		render(UserGroupsPage);
		await vi.waitFor(() => expect(rowKeys()).toEqual([SCIM_GROUP.id?.value ?? '', LOCAL_GROUP.id?.value ?? '']));

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
		expect(rowLinks()).toEqual([
			`/user-groups/${SCIM_GROUP.id?.value ?? ''}`,
			`/user-groups/${LOCAL_GROUP.id?.value ?? ''}`
		]);
	});

	it('round-trips a deep-linked dynamic type filter as is_dynamic=true', async () => {
		mocks.url = new URL('http://localhost/user-groups?type=dynamic&sort=created&sortDir=desc&zoom=list');
		render(UserGroupsPage);
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled());

		const call = lastCall();
		expect(call[ARG.tags]).toEqual({ is_dynamic: 'true' });
		expect(call[ARG.sortField]).toBe(SortField.CREATED_AT);
		expect(call[ARG.sortDir]).toBe(SortDirection.DESC);
	});

	it('disables the delete action for a SCIM-managed group and keeps it enabled otherwise', async () => {
		render(UserGroupsPage);
		await vi.waitFor(() => expect(rowKeys()).toEqual([SCIM_GROUP.id?.value ?? '', LOCAL_GROUP.id?.value ?? '']));

		const triggers = document.querySelectorAll<HTMLButtonElement>('button[aria-label="Actions"]');
		expect(triggers.length).toBe(2);

		triggers[0].click();
		const scimItem = await vi.waitFor(() => {
			const el = document.querySelector('[role="menuitem"]');
			if (!el) throw new Error('SCIM row menu did not open');
			return el;
		});
		expect(scimItem.getAttribute('aria-disabled')).toBe('true');
		expect(scimItem.hasAttribute('data-disabled')).toBe(true);

		document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
		await vi.waitFor(() => {
			if (document.querySelector('[role="menuitem"]')) throw new Error('menu still open');
		});

		triggers[1].click();
		const localItem = await vi.waitFor(() => {
			const el = document.querySelector('[role="menuitem"]');
			if (!el) throw new Error('local row menu did not open');
			return el;
		});
		expect(localItem.getAttribute('aria-disabled')).not.toBe('true');
		expect(localItem.hasAttribute('data-disabled')).toBe(false);
	});
});
