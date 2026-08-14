// The device-groups drill-down: the OVERVIEW is the landing level (one card
// tile per group, every number a ListDeviceGroups response field), and the
// existing list is one zoom in. Clicking a tile zooms to that group's device
// grid — the Devices fleet's existing group level route.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/device-groups'),
	search: vi.fn(),
	listDeviceGroups: vi.fn(),
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
	// The REAL pager: the sweep must actually walk the mocked list RPC.
	const { fetchAllPages } = await import('$lib/sdk/paginate');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			search: mocks.search,
			listDeviceGroups: mocks.listDeviceGroups,
			deleteDeviceGroup: vi.fn()
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import DeviceGroupsPage from './+page.svelte';

const RULE_GROUP = {
	id: '01HZDEVGRPDYNAMIC000000000',
	name: 'Kiosks',
	description: '',
	isDynamic: true,
	memberCount: 7
};
const CURATED_GROUP = {
	id: '01HZDEVGRPSTATIC0000000000',
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
		// Every displayed number is a response field: memberCount, isDynamic.
		const rule = tiles().find((t) => t.dataset.entityId === RULE_GROUP.id)!;
		expect(rule.textContent).toContain('Kiosks');
		expect(rule.textContent).toContain('7 members');
		expect(rule.dataset.dynamic).toBe('true');
		const curated = tiles().find((t) => t.dataset.entityId === CURATED_GROUP.id)!;
		expect(curated.textContent).toContain('3 members');
		expect(curated.dataset.dynamic).toBe('false');
		// The paused list never spends a Search RPC at the landing level.
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile zooms to that group\'s device grid on the Devices fleet', async () => {
		render(DeviceGroupsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.entityId === RULE_GROUP.id)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/devices?zoom=group&group=${RULE_GROUP.id}`);
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
});
