

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/definitions'),
	search: vi.fn(),
	listDefinitions: vi.fn(),
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
			listDefinitions: mocks.listDefinitions,
			listActionSets: vi.fn(async () => ({ sets: [], nextPageToken: '' })),
			deleteDefinition: vi.fn()
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import DefinitionsPage from './+page.svelte';

const CRON_DEF = {
	id: { value: '01HZDEFINITION00000000000A' },
	name: 'Nightly baseline',
	memberCount: 2,
	schedule: { cron: '0 3 * * *', intervalHours: 0, runOnAssign: true, skipIfUnchanged: false }
};
const INTERVAL_DEF = {
	id: { value: '01HZDEFINITION00000000000B' },
	name: 'Workstation rollout',
	memberCount: 5,
	schedule: { cron: '', intervalHours: 12, runOnAssign: false, skipIfUnchanged: true }
};

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="overview-tile"]'));

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	mocks.url = new URL('http://localhost/definitions');
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listDefinitions.mockResolvedValue({
		definitions: [CRON_DEF, INTERVAL_DEF],
		nextPageToken: ''
	});
});

describe('/definitions — the overview is the landing level', () => {
	it('lands on the overview and draws one tile per definition from the list RPC', async () => {
		render(DefinitionsPage);

		await vi.waitFor(() => expect(mocks.listDefinitions).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		expect(document.querySelector('[data-testid="definitions-overview"]')).not.toBeNull();

		const cronTile = tiles().find((t) => t.dataset.entityId === CRON_DEF.id.value)!;
		expect(cronTile.textContent).toContain('0 3 * * *');
		expect(cronTile.textContent).toContain('2 sets');
		const intervalTile = tiles().find((t) => t.dataset.entityId === INTERVAL_DEF.id.value)!;
		expect(intervalTile.textContent).toContain('12');
		expect(intervalTile.textContent).toContain('5 sets');
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile opens the definition\'s existing detail', async () => {
		render(DefinitionsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.entityId === CRON_DEF.id.value)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/definitions/${CRON_DEF.id.value}`);
	});

	it('the level pill zooms to the existing list one level down', async () => {
		render(DefinitionsPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(2), { timeout: 3000 });

		document.querySelector<HTMLButtonElement>('[data-testid="definitions-zoom-list"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull(),
			{ timeout: 3000 }
		);
	});
});
