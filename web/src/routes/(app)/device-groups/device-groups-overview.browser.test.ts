

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/device-groups'),
	search: vi.fn(),
	listDeviceGroups: vi.fn(),
	deleteDeviceGroup: vi.fn(),
	goto: vi.fn()
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
vi.mock('$lib/navigation', () => ({
	goto: (path: string) => mocks.goto(path),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));
vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');

	const { fetchAllPages } = await import('$lib/sdk/paginate');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			search: mocks.search,
			listDeviceGroups: mocks.listDeviceGroups,
			deleteDeviceGroup: mocks.deleteDeviceGroup
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import DeviceGroupsPage from './+page.svelte';

const RULE_GROUP = {
	id: { value: '01HZDEVGRPDYNAMIC000000000' },
	name: 'Kiosks',
	description: '',
	isDynamic: true,
	memberCount: 7
};
const CURATED_GROUP = {
	id: { value: '01HZDEVGRPSTATIC0000000000' },
	name: 'Warehouse',
	description: '',
	isDynamic: false,
	memberCount: 3
};

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="overview-tile"]'));

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	mocks.url = new URL('http://localhost/device-groups');
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listDeviceGroups.mockResolvedValue({
		groups: [RULE_GROUP, CURATED_GROUP],
		nextPageToken: ''
	});
});

describe('/device-groups — the overview is the landing level', () => {
	it('lands on the overview and draws one tile per group from the list RPC', async () => {
		render(DeviceGroupsPage);

		await vi.waitFor(() => expect(mocks.listDeviceGroups).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		expect(document.querySelector('[data-testid="device-groups-overview"]')).not.toBeNull();

		const rule = tiles().find((t) => t.dataset.entityId === RULE_GROUP.id.value)!;
		expect(rule.textContent).toContain('Kiosks');
		expect(rule.textContent).toContain('7 members');
		expect(rule.dataset.dynamic).toBe('true');
		const curated = tiles().find((t) => t.dataset.entityId === CURATED_GROUP.id.value)!;
		expect(curated.textContent).toContain('3 members');
		expect(curated.dataset.dynamic).toBe('false');

		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile zooms to that group\'s device grid on the Devices fleet', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.entityId === RULE_GROUP.id.value)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/devices?zoom=group&group=${RULE_GROUP.id.value}`);
	});

	it('the level pill zooms to the existing list one level down', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		document
			.querySelector<HTMLButtonElement>('[data-testid="device-groups-zoom-list"]')!
			.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull(),
			{ timeout: 3000 }
		);
	});

	it('removes a deleted row by its wrapped id value', async () => {
		const result = create(SearchResultSchema, {
			id: { value: RULE_GROUP.id.value },
			name: RULE_GROUP.name,
			fields: { name: RULE_GROUP.name, member_count: '7', is_dynamic: 'true' }
		});
		let searchResults = [result];
		mocks.search.mockImplementation(async () => ({ results: searchResults, totalCount: searchResults.length }));
		mocks.url = new URL('http://localhost/device-groups?zoom=list');
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(document.querySelectorAll('[data-testid="row-list-row"]')).toHaveLength(1));

		document.querySelector<HTMLButtonElement>('button[aria-label="Actions"]')!.click();
		const menuDelete = await vi.waitFor(() => {
			const item = [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].find(
				(element) => element.textContent?.trim() === m.common_delete()
			);
			expect(item).toBeTruthy();
			return item!;
		});
		menuDelete.click();
		searchResults = [];
		const confirm = await vi.waitFor(() => {
			const button = document.querySelector<HTMLButtonElement>('[data-slot="alert-dialog-action"]');
			expect(button).toBeTruthy();
			return button!;
		});
		confirm.click();
		await vi.waitFor(() => expect(document.querySelectorAll('[data-testid="row-list-row"]')).toHaveLength(0));
		expect(mocks.deleteDeviceGroup).toHaveBeenCalledWith(RULE_GROUP.id.value);
	});
});
