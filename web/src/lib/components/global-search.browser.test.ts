// Conversion contract for the ⌘K palette (movement E).
//
// The palette's whole claim is honesty about what search is: the server does
// prefix matching with keyset paging and no ranking, so entity rows must appear
// EXACTLY as the RPC returned them, paging must ride next_page_token, the facet
// ring must re-scope the RPC rather than filter locally, and only the rows that
// live in this browser tab (windows, terminals, drafts) may be matched locally.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser, userEvent } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { SearchResultSchema } from '$sdk/powermanage/v1/control_pb';
import { SearchScope } from '$sdk/powermanage/v1/common_pb';
import * as m from '$lib/paraglide/messages';
import { Monitor, Send } from '@lucide/svelte';
import {
	shell,
	resetShell,
	openPanel,
	minimizePanel,
	openTerminal,
	addDraft,
	setShellPath
} from '$lib/shell/shell.svelte';
import {
	registerPageSearch,
	activePageSearch,
	resetPageSearch,
	type PageSearchRegistration
} from '$lib/shell/page-search.svelte';

const api = vi.hoisted(() => ({ search: vi.fn() }));
const nav = vi.hoisted(() => ({ goto: vi.fn() }));

// Only the client is faked; the generated protobuf re-exports stay real.
vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	return { ...control, ...common, apiClient: api };
});
vi.mock('$lib/navigation', () => ({ goto: nav.goto, pushState: vi.fn(), replaceState: vi.fn() }));

import GlobalSearch from './global-search.svelte';

const NOW = () => Math.floor(Date.now() / 1000);

function device(id: string, hostname: string, secondsAgo = 5) {
	return create(SearchResultSchema, {
		id,
		name: hostname,
		scope: SearchScope.DEVICES,
		fields: { hostname, last_seen_at: String(NOW() - secondsAgo), os_name: 'Debian' }
	});
}

function auditEvent(id: string, eventType: string) {
	return create(SearchResultSchema, {
		id,
		name: eventType,
		scope: SearchScope.AUDIT_EVENTS,
		fields: { event_type: eventType, actor_type: 'user', occurred_at: String(NOW() - 90) }
	});
}

const empty = { results: [], nextPageToken: '', totalCount: 0 };

function entityRows(): string[] {
	return [...document.querySelectorAll('[data-testid="palette-row"][data-kind="entity"]')].map(
		(el) => el.textContent?.trim() ?? ''
	);
}

beforeEach(() => {
	resetShell();
	resetPageSearch();
	api.search.mockReset();
	api.search.mockResolvedValue(empty);
	nav.goto.mockReset();
});

/** A stand-in for a mounted list page's `createSearchListState`. */
function fakePage(
	scope: number | null,
	label: string
): PageSearchRegistration & { calls: string[] } {
	const calls: string[] = [];
	let value = '';
	return {
		calls,
		scope,
		label: () => label,
		get query() {
			return value;
		},
		setQuery(next: string) {
			calls.push(next);
			value = next;
		},
		clear() {
			calls.push('<clear>');
			value = '';
		}
	};
}

const activeFacet = () =>
	document.querySelector('[data-testid="palette-facet"][aria-pressed="true"]');

async function openPalette(query?: string) {
	render(GlobalSearch, { open: true });
	const box = browser.getByRole('combobox');
	await expect.element(box).toBeVisible();
	if (query !== undefined) {
		await box.fill(query);
		await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });
	}
	return box;
}

describe('palette — entity rows are the server’s answer, verbatim', () => {
	it('renders results in response order and never re-sorts them', async () => {
		// Deliberately NOT alphabetical: a palette that ranked or sorted would
		// reorder these, and this is the only signal that it does not.
		const shuffled = [
			device('01JQZZ0000000000000000000C', 'api-prod-04'),
			device('01JQZZ0000000000000000000A', 'api-prod-01'),
			device('01JQZZ0000000000000000000B', 'api-prod-02')
		];
		api.search.mockResolvedValue({ results: shuffled, nextPageToken: '', totalCount: 3 });

		await openPalette('api');

		await vi.waitFor(() => expect(entityRows()).toHaveLength(3), { timeout: 3000 });
		expect(entityRows().map((t) => t.split(' ')[0])).toEqual([
			'api-prod-04',
			'api-prod-01',
			'api-prod-02'
		]);
	});

	it('sends the raw query, one page at a time, with no scope by default', async () => {
		await openPalette('a');
		expect(api.search).toHaveBeenLastCalledWith('a', SearchScope.UNSPECIFIED, 8, '');
	});

	it('opens a result on its own detail route', async () => {
		api.search.mockResolvedValue({
			results: [device('01JQZZ0000000000000000000A', 'api-prod-01')],
			nextPageToken: '',
			totalCount: 1
		});
		await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(1), { timeout: 3000 });

		await browser.getByTestId('palette-row').first().click();
		expect(nav.goto).toHaveBeenCalledWith('/devices/01JQZZ0000000000000000000A');
	});

	it('deep-links the section query for a scope that has no detail page', async () => {
		api.search.mockResolvedValue({
			results: [auditEvent('01JQZZ0000000000000000000D', 'device.registered')],
			nextPageToken: '',
			totalCount: 1
		});
		await openPalette('device');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(1), { timeout: 3000 });

		await browser.getByTestId('palette-row').first().click();
		expect(nav.goto).toHaveBeenCalledWith('/audit?query=device.registered');
	});
});

describe('palette — status is never colour alone', () => {
	it('names a device row’s status in words, not only in the tone dot', async () => {
		api.search.mockResolvedValue({
			results: [device('01JQZZ0000000000000000000A', 'api-prod-01', 5)],
			nextPageToken: '',
			totalCount: 1
		});
		await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(1), { timeout: 3000 });

		// Seen 5 s ago: inside the heartbeat window.
		const dot = document.querySelector('[data-testid="palette-dot"]');
		expect(dot?.getAttribute('data-tone')).toBe('ok');
		expect(dot?.getAttribute('role')).toBe('img');
		expect(dot?.getAttribute('aria-label')).toBe(m.fleet_tile_ok());
		// …and the same word is in the row's own text, so the status survives a
		// screen that renders no colour at all.
		expect(entityRows()[0]).toContain(m.fleet_tile_ok());
	});

	it('calls a device that missed the heartbeat window unreachable, not "never seen"', async () => {
		api.search.mockResolvedValue({
			results: [device('01JQZZ0000000000000000000B', 'api-prod-02', 900)],
			nextPageToken: '',
			totalCount: 1
		});
		await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(1), { timeout: 3000 });

		const dot = document.querySelector('[data-testid="palette-dot"]');
		expect(dot?.getAttribute('data-tone')).toBe('crit');
		expect(dot?.getAttribute('aria-label')).toBe(m.fleet_tile_crit());
		expect(entityRows()[0]).toContain(m.fleet_tile_crit());
		expect(entityRows()[0]).not.toContain(m.fleet_tile_idle());
	});
});

describe('palette — the facet ring re-scopes the RPC', () => {
	it('cycles scopes with ⇥ and asks the server again for each one', async () => {
		await openPalette('api');
		expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.UNSPECIFIED, 8, '');

		await userEvent.keyboard('{Tab}');
		await vi.waitFor(
			() => expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.DEVICES, 8, ''),
			{ timeout: 3000 }
		);

		await userEvent.keyboard('{Tab}');
		await vi.waitFor(
			() => expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.DEVICE_GROUPS, 8, ''),
			{ timeout: 3000 }
		);

		// ⇧⇥ walks back, and the chip row marks exactly one active facet.
		await userEvent.keyboard('{Shift>}{Tab}{/Shift}');
		await vi.waitFor(
			() => expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.DEVICES, 8, ''),
			{ timeout: 3000 }
		);
		const pressed = [...document.querySelectorAll('[data-testid="palette-facet"]')].filter(
			(el) => el.getAttribute('aria-pressed') === 'true'
		);
		expect(pressed).toHaveLength(1);
		expect(pressed[0].getAttribute('data-scope')).toBe(String(SearchScope.DEVICES));
	});

	it('claims a total only for the scope the request actually carried', async () => {
		const rows = [
			device('01JQZZ0000000000000000000A', 'api-prod-01'),
			device('01JQZZ0000000000000000000B', 'api-prod-02')
		];
		api.search.mockResolvedValue({ results: rows, nextPageToken: '', totalCount: 41 });

		// Under "All" the total spans every scope, so the group must not claim it.
		await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(2), { timeout: 3000 });
		await expect
			.element(
				browser.getByText(
					m.search_group_count({ scope: m.search_group_devices(), query: 'api', shown: 2 })
				)
			)
			.toBeVisible();

		// Scoped to devices, "2 of 41" is true.
		await userEvent.keyboard('{Tab}');
		await expect
			.element(
				browser.getByText(
					m.search_group_count_total({
						scope: m.search_group_devices(),
						query: 'api',
						shown: 2,
						total: 41
					})
				)
			)
			.toBeVisible();
	});
});

describe('palette — paging is keyset, not pages', () => {
	it('shows the next page only when the server handed back a cursor, and appends it', async () => {
		const first = [
			device('01JQZZ0000000000000000000A', 'api-prod-01'),
			device('01JQZZ0000000000000000000B', 'api-prod-02')
		];
		const second = [
			device('01JQZZ0000000000000000000C', 'api-prod-03'),
			device('01JQZZ0000000000000000000D', 'api-prod-04')
		];
		api.search.mockResolvedValueOnce({ results: first, nextPageToken: 'cursor-1', totalCount: 4 });
		api.search.mockResolvedValueOnce({ results: second, nextPageToken: '', totalCount: 4 });

		await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(2), { timeout: 3000 });
		await expect.element(browser.getByTestId('palette-show-next')).toBeVisible();

		await browser.getByTestId('palette-show-next').click();
		await vi.waitFor(() => expect(entityRows()).toHaveLength(4), { timeout: 3000 });
		expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.UNSPECIFIED, 8, 'cursor-1');
		expect(entityRows().map((t) => t.split(' ')[0])).toEqual([
			'api-prod-01',
			'api-prod-02',
			'api-prod-03',
			'api-prod-04'
		]);
		// Cursor spent: no page row remains.
		expect(document.querySelector('[data-testid="palette-show-next"]')).toBeNull();
	});

	it('offers no paging row when the response carries no cursor', async () => {
		api.search.mockResolvedValue({
			results: [device('01JQZZ0000000000000000000A', 'api-prod-01')],
			nextPageToken: '',
			totalCount: 1
		});
		await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(1), { timeout: 3000 });
		expect(document.querySelector('[data-testid="palette-show-next"]')).toBeNull();
	});
});

describe('palette — "This shell" restores what lives in this tab', () => {
	it('restores a stashed window, focuses a terminal and resumes a draft', async () => {
		const panelId = openPanel('device', 'dev-1', 'api-prod-01');
		minimizePanel(panelId);
		const terminalId = openTerminal('dev-2', 'db-staging-01');
		shell.terminal.activeId = null;
		let resumed = 0;
		// The owner is the mounted surface, so this draft resumes IN PLACE; the
		// palette's cross-route path (navigate home) is covered below.
		setShellPath('/assign');
		addDraft({
			id: 'draft:assign',
			contextId: 'assign',
			kind: 'draft',
			title: 'Assign · 12 devices',
			subtitle: 'Patch & reboot v7',
			route: '/assign',
			onRestore: () => resumed++
		});

		// No query: the shell group stands alone and no RPC is issued for it.
		await openPalette();
		await expect.element(browser.getByText(m.search_group_shell())).toBeVisible();
		expect(api.search).not.toHaveBeenCalled();

		await browser.getByTestId('palette-row').nth(0).click();
		expect(shell.panels.find((p) => p.id === panelId)?.minimized).toBe(false);

		render(GlobalSearch, { open: true });
		await browser.getByText('db-staging-01').click();
		expect(shell.terminal.activeId).toBe(terminalId);
		expect(shell.terminal.open).toBe(true);

		render(GlobalSearch, { open: true });
		await browser.getByText('Assign · 12 devices').click();
		expect(resumed).toBe(1);
	});

	it('a draft whose surface is gone NAVIGATES home instead of reviving a dead context', async () => {
		let resumed = 0;
		setShellPath('/devices'); // the operator is somewhere else entirely
		addDraft({
			id: 'draft:assign',
			contextId: 'assign',
			kind: 'draft',
			title: 'Assign · 12 devices',
			route: '/assign',
			onRestore: () => resumed++
		});

		await openPalette();
		await browser.getByText('Assign · 12 devices').click();

		expect(nav.goto).toHaveBeenCalledWith('/assign');
		// Untouched closures: the dead context is never revived from here.
		expect(resumed).toBe(0);
		// The card pops on the click; the buffer is staged for the surface to
		// claim when it mounts. A card that lingered until some later claim was
		// the "restoring does not remove it reliably" bug.
		expect(shell.drafts).toHaveLength(0);
	});

	it('matches shell rows locally — and only shell rows', async () => {
		openPanel('device', 'dev-1', 'api-prod-01');
		openTerminal('dev-2', 'db-staging-01');
		api.search.mockResolvedValue(empty);

		await openPalette('api');

		// The server returned nothing, yet the local window still matches...
		await expect.element(browser.getByText('api-prod-01')).toBeVisible();
		// ...and the terminal, which does not match the query, does not.
		expect(document.body.textContent).not.toContain('db-staging-01');
	});
});

describe('palette — keyboard and dismissal', () => {
	it('walks rows with the arrows, opens with ↵ and closes with Esc', async () => {
		api.search.mockResolvedValue({
			results: [
				device('01JQZZ0000000000000000000A', 'api-prod-01'),
				device('01JQZZ0000000000000000000B', 'api-prod-02')
			],
			nextPageToken: '',
			totalCount: 2
		});
		const box = await openPalette('api');
		await vi.waitFor(() => expect(entityRows()).toHaveLength(2), { timeout: 3000 });

		const active = () => box.element().getAttribute('aria-activedescendant');
		expect(active()).toBe('entity-5-01JQZZ0000000000000000000A');

		await userEvent.keyboard('{ArrowDown}');
		await vi.waitFor(() => expect(active()).toBe('entity-5-01JQZZ0000000000000000000B'));
		await userEvent.keyboard('{Enter}');
		expect(nav.goto).toHaveBeenCalledWith('/devices/01JQZZ0000000000000000000B');
		expect(document.querySelector('[data-testid="global-search"]')).toBeNull();

		render(GlobalSearch, { open: true });
		await expect.element(browser.getByRole('combobox')).toBeVisible();
		await userEvent.keyboard('{Escape}');
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="global-search"]')).toBeNull()
		);
	});

	it('states the keyboard and search contract in the footer', async () => {
		await openPalette();
		await expect.element(browser.getByText(m.search_footer_keys())).toBeVisible();
		await expect.element(browser.getByText(m.search_footer_contract())).toBeVisible();
	});
});

// ── the unified surface: shell jump targets live here now ───────────────────
describe('palette — one surface for entities AND the shell', () => {
	const SECTIONS = [
		{ href: '/devices', label: () => 'Devices', icon: Monitor },
		{ href: '/actions', label: () => 'Actions', icon: Send }
	];
	const OVERFLOW = [
		{ group: () => 'Admin', items: [{ href: '/users', label: () => 'Users', icon: Monitor }] }
	];

	/** Section-row titles only — the facet chips share these words. */
	const sectionRows = () =>
		[...document.querySelectorAll('[data-testid="palette-row"][data-kind="section"]')].map(
			(el) => el.querySelector('span')?.textContent?.trim() ?? ''
		);

	it('lists sections (pill AND overflow) and navigates on activation', async () => {
		render(GlobalSearch, { open: true, sections: SECTIONS, overflow: OVERFLOW });
		await expect.element(browser.getByText(m.search_group_goto())).toBeVisible();
		expect(sectionRows()).toEqual(['Devices', 'Actions', 'Users']);

		const row = [...document.querySelectorAll('[data-testid="palette-row"][data-kind="section"]')].find(
			(el) => el.textContent?.includes('Actions')
		);
		(row as HTMLElement).click();
		// $lib/navigation.goto owns the base prefix — the palette must not add a
		// second one, so the row hands it the app-relative href verbatim.
		expect(nav.goto).toHaveBeenCalledWith('/actions');
	});

	it('matches sections locally, without asking the server for them', async () => {
		render(GlobalSearch, { open: true, sections: SECTIONS, overflow: OVERFLOW });
		const box = browser.getByRole('combobox');
		await box.fill('user');
		await vi.waitFor(() => expect(sectionRows()).toEqual(['Users']));
	});
});

// ── page-scoped ⌘K (the operator searches what they are looking at) ─────────
describe('palette — scoped to the page the operator is on', () => {
	it('preselects the page’s facet and types into the PAGE’s list, not the RPC', async () => {
		const actions = fakePage(SearchScope.ACTIONS, 'Actions');
		registerPageSearch(actions);

		const box = await openPalette();
		// The leading chip is the page itself, and it is the pressed one.
		const chip = activeFacet();
		expect(chip?.getAttribute('data-page-facet')).toBe('true');
		expect(chip?.getAttribute('data-scope')).toBe(String(SearchScope.ACTIONS));
		expect(chip?.textContent?.trim()).toBe('Actions');
		await expect.element(browser.getByTestId('palette-page-row')).toBeVisible();

		await box.fill('api');
		// The page's own setSearch ran…
		await vi.waitFor(() => expect(actions.query).toBe('api'));
		// …and nothing was searched or navigated behind the operator's back.
		expect(api.search).not.toHaveBeenCalled();
		expect(nav.goto).not.toHaveBeenCalled();
		expect(document.querySelectorAll('[data-testid="palette-row"][data-kind="entity"]')).toHaveLength(0);
	});

	it('↵ on the page row only dismisses — the filter stays applied', async () => {
		const actions = fakePage(SearchScope.ACTIONS, 'Actions');
		registerPageSearch(actions);
		const box = await openPalette();
		await box.fill('api');
		await vi.waitFor(() => expect(actions.query).toBe('api'));
		const written = actions.calls.length;

		await userEvent.keyboard('{Enter}');
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="global-search"]')).toBeNull()
		);
		// never cleared, never re-issued
		expect(actions.calls.length).toBe(written);
		expect(actions.query).toBe('api');
	});

	it('offers a clear row that drops the page’s query', async () => {
		const actions = fakePage(SearchScope.ACTIONS, 'Actions');
		registerPageSearch(actions);
		const box = await openPalette();
		expect(document.querySelector('[data-testid="palette-page-clear"]')).toBeNull();

		await box.fill('api');
		await expect.element(browser.getByTestId('palette-page-clear')).toBeVisible();
		await browser.getByTestId('palette-page-clear').click();
		expect(actions.calls.at(-1)).toBe('<clear>');
		expect(actions.query).toBe('');
	});

	it('⇥ off the page facet turns it back into the global palette', async () => {
		const actions = fakePage(SearchScope.ACTIONS, 'Actions');
		registerPageSearch(actions);

		const box = await openPalette();
		await box.fill('api');
		await vi.waitFor(() => expect(actions.query).toBe('api'));
		expect(api.search).not.toHaveBeenCalled(); // positive control
		const written = actions.calls.length;

		// The ring's second entry is "All" — the global palette's resting facet.
		await userEvent.keyboard('{Tab}');
		await vi.waitFor(
			() => expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.UNSPECIFIED, 8, ''),
			{ timeout: 3000 }
		);
		expect(activeFacet()?.getAttribute('data-page-facet')).toBeNull();

		// Typing now belongs to the server, not to the page.
		await box.fill('apix');
		await vi.waitFor(
			() => expect(api.search).toHaveBeenLastCalledWith('apix', SearchScope.UNSPECIFIED, 8, ''),
			{ timeout: 3000 }
		);
		expect(actions.calls.length).toBe(written); // the page saw nothing after ⇥
		expect(actions.query).toBe('api');
	});

	it('Esc leaves the page’s query exactly as it stands', async () => {
		const actions = fakePage(SearchScope.ACTIONS, 'Actions');
		registerPageSearch(actions);

		const box = await openPalette();
		await box.fill('api');
		await vi.waitFor(() => expect(actions.query).toBe('api'));

		const written = actions.calls.length;
		await userEvent.keyboard('{Escape}');
		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="global-search"]')).toBeNull()
		);
		expect(actions.query).toBe('api');
		expect(actions.calls.length).toBe(written);

		// Re-opening seeds the box from the page, so the operator resumes where
		// they left off instead of facing an empty field over a filtered list.
		render(GlobalSearch, { open: true });
		await expect.element(browser.getByRole('combobox')).toHaveValue('api');
	});

	it('a page with no Search scope still leads the ring with its own chip', async () => {
		const roles = fakePage(null, 'Roles');
		registerPageSearch(roles);

		const box = await openPalette();
		expect(activeFacet()?.getAttribute('data-page-facet')).toBe('true');
		expect(activeFacet()?.textContent?.trim()).toBe('Roles');

		await box.fill('adm');
		await vi.waitFor(() => expect(roles.query).toBe('adm'));
		expect(api.search).not.toHaveBeenCalled();
	});

	it('with no page registered, ⌘K is the global palette exactly as before', async () => {
		const box = await openPalette();
		// No page chip at all, and "All" leads the ring.
		expect(document.querySelector('[data-testid="palette-facet"][data-page-facet]')).toBeNull();
		expect(activeFacet()?.getAttribute('data-scope')).toBe(String(SearchScope.UNSPECIFIED));
		expect(document.querySelector('[data-testid="palette-page-row"]')).toBeNull();

		await box.fill('api');
		await vi.waitFor(() => expect(api.search).toHaveBeenLastCalledWith('api', SearchScope.UNSPECIFIED, 8, ''), {
			timeout: 3000
		});
	});

	// The stale-scope guard, end to end: a released page must not still own ⌘K.
	it('a released page is not inherited by the next one', async () => {
		const actions = fakePage(SearchScope.ACTIONS, 'Actions');
		const release = registerPageSearch(actions);
		// positive control — the seam really carried this page, so the assertions
		// below are a withdrawal and not an empty seam.
		expect(activePageSearch()).toBe(actions);

		release(); // the actions page unmounts
		expect(activePageSearch()).toBeNull();

		const box = await openPalette();
		expect(document.querySelector('[data-testid="palette-facet"][data-page-facet]')).toBeNull();
		expect(activeFacet()?.getAttribute('data-scope')).toBe(String(SearchScope.UNSPECIFIED));

		await box.fill('api');
		await vi.waitFor(() => expect(api.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(actions.calls).toEqual([]); // the dead page never saw a keystroke
	});
});
