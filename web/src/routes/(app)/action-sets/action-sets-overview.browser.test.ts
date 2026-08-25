

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/action-sets'),
	search: vi.fn(),
	listActionSets: vi.fn(),
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
			listActionSets: mocks.listActionSets,
			listActions: vi.fn(async () => ({ actions: [], nextPageToken: '' })),
			deleteActionSet: vi.fn()
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import ActionSetsPage from './+page.svelte';

const BASE_SET = { id: '01HZACTSET000000000000000A', name: 'Base System Setup', memberCount: 4 };
const EMPTY_SET = { id: '01HZACTSET000000000000000B', name: 'Quarantine', memberCount: 0 };

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="overview-tile"]'));

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	mocks.url = new URL('http://localhost/action-sets');
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listActionSets.mockResolvedValue({ sets: [BASE_SET, EMPTY_SET], nextPageToken: '' });
});

describe('/action-sets — the overview is the landing level', () => {
	it('lands on the overview and draws one tile per set from the list RPC', async () => {
		render(ActionSetsPage);

		await vi.waitFor(() => expect(mocks.listActionSets).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		expect(document.querySelector('[data-testid="action-sets-overview"]')).not.toBeNull();
		const base = tiles().find((t) => t.dataset.entityId === BASE_SET.id)!;
		expect(base.textContent).toContain('Base System Setup');

		expect(base.textContent).toContain('4 actions');
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile opens the set\'s existing detail', async () => {
		render(ActionSetsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.entityId === BASE_SET.id)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/action-sets/${BASE_SET.id}`);
	});

	it('the level pill zooms to the existing list one level down', async () => {
		render(ActionSetsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		document.querySelector<HTMLButtonElement>('[data-testid="action-sets-zoom-list"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull(),
			{ timeout: 3000 }
		);
	});
});
