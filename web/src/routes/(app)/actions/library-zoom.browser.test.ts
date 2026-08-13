// The action library's semantic zoom, exercised through the REAL route component
// against mocked RPCs. What is load-bearing here:
//
//   1. the bubbles are the sweep's answer — one per type really present, with
//      counts and per-action tiles derived from ManagedAction fields only;
//   2. a sweep that hits its bound SAYS it is partial instead of implying the
//      tiles are the whole library;
//   3. clicking a type drives the page's REAL type filter — the same
//      `?types=` param and the same tag filter the menu produces, never a fork;
//   4. the overview never spends a Search RPC (the fleet's promise), while the
//      pill's page-search registration survives BOTH levels;
//   5. zoom round-trips through the URL, so an overview is a link.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ManagedActionSchema } from '$sdk/powermanage/v1/control_pb';
import { ActionType } from '$sdk/powermanage/v1/actions_pb';
import { DesiredState, SearchScope } from '$sdk/powermanage/v1/common_pb';
import * as m from '$lib/paraglide/messages';

const mocks = vi.hoisted(() => ({
	url: new URL('https://control.test/actions'),
	search: vi.fn(),
	listActions: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn()
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
	pushState: mocks.pushState,
	replaceState: mocks.replaceState,
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

// Only the client is faked; the generated protobuf re-exports stay real, so the
// page's ActionType / DesiredState / SearchScope constants are the production ones.
vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: { search: mocks.search, listActions: mocks.listActions, deleteAction: vi.fn() },
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn()
	};
});

import ActionsPage from './+page.svelte';
import { activePageSearch, resetPageSearch } from '$lib/shell/page-search.svelte';
import { MAX_PAGES, PAGE_SIZE } from './library-data';

function action(o: {
	id: string;
	name: string;
	type?: ActionType;
	absent?: boolean;
	compliance?: boolean;
}) {
	const base = {
		id: o.id,
		name: o.name,
		type: o.type ?? ActionType.SHELL,
		desiredState: o.absent ? DesiredState.ABSENT : DesiredState.PRESENT
	};
	if (o.compliance === undefined) return create(ManagedActionSchema, base);
	return create(ManagedActionSchema, {
		...base,
		params: { case: 'shell' as const, value: { script: 'true', isCompliance: o.compliance } }
	});
}

const LIBRARY = [
	action({ id: 'p1', name: 'Install Firefox', type: ActionType.PACKAGE }),
	action({ id: 'p2', name: 'Drop telnet', type: ActionType.PACKAGE, absent: true }),
	action({ id: 's1', name: 'Rotate logs', type: ActionType.SHELL, compliance: false }),
	action({ id: 'c1', name: 'Check LUKS', type: ActionType.SHELL, compliance: true })
];

const bubbles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="library-bubble"]'));
const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="library-tile"]'));
const stats = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="fleet-stat"]')).map((s) => [
		s.dataset.tone,
		s.querySelector('b')!.textContent
	]);

/** Positional apiClient.search args — see list-logic's SearchArgs tuple. */
function lastSearchArgs() {
	const call = mocks.search.mock.calls.at(-1);
	if (!call) throw new Error('the page never issued a search');
	const [query, scope, pageSize, pageToken, dateFilters, tagFilters, sortField, sortDirection] =
		call;
	return { query, scope, pageSize, pageToken, dateFilters, tagFilters, sortField, sortDirection };
}

/** The URL every history write in this test carries, in the order they landed. */
function pushedTargets(): string[] {
	return mocks.pushState.mock.calls.map((c) => String(c[0]));
}

beforeEach(() => {
	document.body.innerHTML = '';
	mocks.url = new URL('https://control.test/actions?zoom=overview');
	mocks.search.mockReset();
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listActions.mockReset();
	mocks.listActions.mockResolvedValue({ actions: LIBRARY, nextPageToken: '', totalCount: 4 });
	mocks.pushState.mockReset();
	mocks.replaceState.mockReset();
	resetPageSearch();
});

describe('overview zoom — the bubbles are the sweep’s answer', () => {
	it('derives one bubble per type present, with real counts and tiles', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });

		// A compliance-flagged SHELL action is its own bucket, not a shell script.
		expect(bubbles().map((b) => b.dataset.bucket)).toEqual(['package', 'compliance', 'shell']);
		expect(bubbles().map((b) => b.querySelectorAll('[data-testid="library-tile"]').length)).toEqual([
			2, 1, 1
		]);
		// One tile per swept action, no more.
		expect(tiles().length).toBe(LIBRARY.length);

		// The removal carries its own shape, so the state is never colour-alone.
		const removal = tiles().find((t) => t.dataset.actionId === 'p2');
		expect(removal).toBeDefined();
		expect(removal!.dataset.state).toBe('absent');
		expect(removal!.getAttribute('aria-label')).toBe(
			`Drop telnet · ${m.desired_state_absent()}`
		);
		expect(removal!.querySelector('[data-marker="notch"]')).not.toBeNull();
	});

	it('summarises the same partition the tiles were painted from', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });

		// install 3 · remove 1 · 3 types — install + remove is the swept total.
		expect(stats()).toEqual([
			['ok', '3'],
			['crit', '1'],
			['info', '3']
		]);
	});

	it('sweeps ListActions at the contract’s page size and stops when exhausted', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });

		expect(mocks.listActions).toHaveBeenCalledTimes(1);
		expect(mocks.listActions).toHaveBeenCalledWith(PAGE_SIZE, '');
	});

	it('says the library is partial when the sweep hits its bound', async () => {
		// A server that never stops handing out page tokens, one distinct action
		// per page — the sweep must stop at MAX_PAGES rather than follow forever.
		let n = 0;
		mocks.listActions.mockImplementation(async () => ({
			actions: [action({ id: `page-${n++}`, name: `Action ${n}`, type: ActionType.PACKAGE })],
			nextPageToken: 'more',
			totalCount: 9000
		}));
		render(ActionsPage);

		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="library-truncated"]')).not.toBeNull(),
			{ timeout: 5000 }
		);
		expect(mocks.listActions).toHaveBeenCalledTimes(MAX_PAGES);
		expect(tiles().length).toBe(MAX_PAGES);
		const banner = document.querySelector('[data-testid="library-truncated"]')!.textContent!;
		expect(banner).toContain(String(MAX_PAGES)); // what it really shows
		expect(banner).toContain('9000'); // what the server says exists
	});

	it('counts an action once when two pages of the sweep overlap', async () => {
		// Rows shift between pages; a repeated ULID must not inflate the counts
		// (nor blow up the keyed tile grid).
		mocks.listActions
			.mockResolvedValueOnce({ actions: LIBRARY, nextPageToken: 'next', totalCount: 4 })
			.mockResolvedValueOnce({
				actions: [LIBRARY[0], action({ id: 'p9', name: 'Install vim', type: ActionType.PACKAGE })],
				nextPageToken: '',
				totalCount: 5
			});
		render(ActionsPage);

		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });
		expect(tiles().length).toBe(5);
		expect(tiles().filter((t) => t.dataset.actionId === 'p1').length).toBe(1);
	});

	it('shows the honest empty state, not an empty grid, when the library is empty', async () => {
		mocks.listActions.mockResolvedValue({ actions: [], nextPageToken: '', totalCount: 0 });
		render(ActionsPage);

		await vi.waitFor(() => expect(document.body.textContent).toContain(m.actions_empty()), {
			timeout: 3000
		});
		expect(bubbles().length).toBe(0);
		expect(tiles().length).toBe(0);
	});
});

describe('clicking a type drives the page’s real filter', () => {
	it('lands on the list narrowed by the same ?types= param the menu writes', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });
		expect(mocks.search).not.toHaveBeenCalled();

		const pkg = bubbles().find((b) => b.dataset.bucket === 'package')!;
		pkg.querySelector<HTMLButtonElement>('[data-testid="library-bubble-header"]')!.click();

		// One gesture, one history entry — carrying BOTH the level and the filter.
		await vi.waitFor(() => expect(pushedTargets().length).toBe(1));
		expect(pushedTargets()[0]).toContain('types=package');
		expect(pushedTargets()[0]).not.toContain('zoom=overview');

		// …and the list it lands on really asks the server for that type.
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(lastSearchArgs().tagFilters).toEqual({ type: String(ActionType.PACKAGE) });
		expect(lastSearchArgs().scope).toBe(SearchScope.ACTIONS);
	});

	it('asks for the compliance tag, not a shell type, from the compliance bubble', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });

		const compliance = bubbles().find((b) => b.dataset.bucket === 'compliance')!;
		compliance.querySelector<HTMLButtonElement>('[data-testid="library-bubble-header"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(lastSearchArgs().tagFilters).toEqual({ is_compliance: 'true' });
	});

	it('offers no click on a type the page’s filter cannot name', async () => {
		mocks.listActions.mockResolvedValue({
			actions: [action({ id: 'w1', name: 'Corp wifi', type: ActionType.WIFI })],
			nextPageToken: '',
			totalCount: 1
		});
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBe(1), { timeout: 3000 });

		const bubble = bubbles()[0];
		expect(bubble.dataset.filterable).toBe('false');
		expect(bubble.querySelector('[data-testid="library-bubble-header"]')).toBeNull();
		expect(bubble.textContent).toContain(m.actions_overview_no_filter());
	});
});

describe('the overview never spends a search, and ⌘K keeps its facet', () => {
	it('never fires the search RPC at a level that does not render rows', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(tiles().length).toBeGreaterThan(0), { timeout: 3000 });

		// give the search state's debounce more than enough room to have fired
		await new Promise((r) => setTimeout(r, 500));
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('keeps this page registered as the pill’s search scope at BOTH levels', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(tiles().length).toBeGreaterThan(0), { timeout: 3000 });

		// overview: registered, and typing into it still costs no RPC
		expect(activePageSearch()?.scope).toBe(SearchScope.ACTIONS);
		activePageSearch()!.setQuery('firefox');
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="library-query-note"]')).not.toBeNull()
		);
		expect(activePageSearch()!.query).toBe('firefox');
		await new Promise((r) => setTimeout(r, 500));
		expect(mocks.search).not.toHaveBeenCalled();

		// list: same registration, and the query it collected reaches the server
		document.querySelector<HTMLButtonElement>('[data-testid="library-zoom-list"]')!.click();
		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(activePageSearch()?.scope).toBe(SearchScope.ACTIONS);
		expect(lastSearchArgs().query).toBe('firefox');
	});
});

describe('zoom is URL state', () => {
	// The drill-down grammar: the OVERVIEW is the landing level, the list is one
	// zoom in — same as the Devices fleet. An explicit level in the URL still wins.
	it('defaults to the overview: a bare /actions link lands on the high level', async () => {
		mocks.url = new URL('https://control.test/actions');
		render(ActionsPage);

		// Landing pays for the sweep, not for list rows.
		await vi.waitFor(() => expect(mocks.listActions).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });
		expect(
			document
				.querySelector<HTMLElement>('[data-testid="library-zoom-overview"]')!
				.getAttribute('aria-pressed')
		).toBe('true');
		// The paused list never spends a Search RPC at the landing level.
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('an explicit ?zoom=list deep link still lands on the list', async () => {
		mocks.url = new URL('https://control.test/actions?zoom=list');
		render(ActionsPage);

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(
			document
				.querySelector<HTMLElement>('[data-testid="library-zoom-list"]')!
				.getAttribute('aria-pressed')
		).toBe('true');
		expect(bubbles().length).toBe(0);
		// The sweep stays lazy: the list level never pays for it.
		expect(mocks.listActions).not.toHaveBeenCalled();
	});

	it('pushes the level the operator picks and renders the level the URL names', async () => {
		mocks.url = new URL('https://control.test/actions');
		render(ActionsPage);
		await vi.waitFor(() => expect(bubbles().length).toBeGreaterThan(0), { timeout: 3000 });

		document.querySelector<HTMLButtonElement>('[data-testid="library-zoom-list"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(pushedTargets().at(-1)).toContain('zoom=list');
		expect(
			document
				.querySelector<HTMLElement>('[data-testid="library-zoom-list"]')!
				.getAttribute('aria-pressed')
		).toBe('true');
	});

	it('carries the tour anchors the surface exposes', async () => {
		render(ActionsPage);
		await vi.waitFor(() => expect(tiles().length).toBeGreaterThan(0), { timeout: 3000 });

		for (const anchor of ['library-zoom', 'library-summary', 'library-grid']) {
			expect(document.querySelector(`[data-tour="${anchor}"]`), anchor).not.toBeNull();
		}
	});
});
