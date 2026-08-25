

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/user-groups'),
	search: vi.fn(),
	listUserGroups: vi.fn(),
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
			listUserGroups: mocks.listUserGroups,
			deleteUserGroup: vi.fn()
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import UserGroupsPage from './+page.svelte';

const RULE_GROUP = {
	id: { value: '01HZUSRGRPDYNAMIC000000000' },
	name: 'Berlin staff',
	description: '',
	dynamicQuery: 'user.disabled == false',
	isScimManaged: false,
	memberCount: 12
};
const CURATED_GROUP = {
	id: { value: '01HZUSRGRPSTATIC0000000000' },
	name: 'Operators',
	description: '',
	dynamicQuery: undefined,
	isScimManaged: false,
	memberCount: 2
};

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="overview-tile"]'));

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	mocks.url = new URL('http://localhost/user-groups');
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listUserGroups.mockResolvedValue({
		groups: [RULE_GROUP, CURATED_GROUP],
		nextPageToken: ''
	});
});

describe('/user-groups — the overview is the landing level', () => {
	it('lands on the overview and draws one tile per group from the list RPC', async () => {
		render(UserGroupsPage);

		await vi.waitFor(() => expect(mocks.listUserGroups).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		expect(document.querySelector('[data-testid="user-groups-overview"]')).not.toBeNull();
		const rule = tiles().find((t) => t.dataset.entityId === RULE_GROUP.id.value)!;
		expect(rule.textContent).toContain('Berlin staff');
		expect(rule.textContent).toContain('12 members');
		expect(rule.dataset.dynamic).toBe('true');
		const curated = tiles().find((t) => t.dataset.entityId === CURATED_GROUP.id.value)!;
		expect(curated.textContent).toContain('2 members');
		expect(curated.dataset.dynamic).toBe('false');
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile opens the group\'s existing detail', async () => {
		render(UserGroupsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.entityId === CURATED_GROUP.id.value)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/user-groups/${CURATED_GROUP.id.value}`);
	});

	it('the level pill zooms to the existing list one level down', async () => {
		render(UserGroupsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		document.querySelector<HTMLButtonElement>('[data-testid="user-groups-zoom-list"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull(),
			{ timeout: 3000 }
		);
	});
});
