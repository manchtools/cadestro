

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import * as m from '$lib/paraglide/messages';

const api = vi.hoisted(() => ({
	createDefinition: vi.fn(),
	deleteDefinition: vi.fn(),
	getDefinition: vi.fn(),
	listActionSets: vi.fn(),
	search: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/definitions/new') }));

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
		persistDraft: () => {},
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
import NewDefinitionPage from './+page.svelte';
import DefinitionsPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/definitions/new';
const DEF_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/definitions/new');
	api.createDefinition.mockResolvedValue({ id: { value: DEF_ID }, name: 'Baseline' });
	api.search.mockResolvedValue({ results: [], totalCount: 0n, nextPageToken: '' });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);
const area = (id: string) => document.querySelector<HTMLTextAreaElement>(`#${id}`);

function type(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillDefinition(name: string, description: string) {
	await vi.waitFor(() => expect(field('definition-name')).toBeTruthy());
	type(field('definition-name')!, name);
	type(area('definition-description')!, description);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/definitions/new — the commit is the pill\'s', () => {
	it('declares a route, which is what earns the Stash button', async () => {
		render(NewDefinitionPage);
		await fillDefinition('Baseline', 'Everything a laptop needs');

		expect(shell.pill.context?.route).toBe(ROUTE);

		expect(shell.pill.context?.commitLabel).toBe(m.common_create());
	});

	it('creates the definition with the exact request the dialog sent', async () => {
		render(NewDefinitionPage);
		await fillDefinition('  Baseline  ', '  Everything a laptop needs  ');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createDefinition).toHaveBeenCalledTimes(1));
		const request = api.createDefinition.mock.calls[0][0];
		expect(request.name).toBe('Baseline');
		expect(request.description).toBe('Everything a laptop needs');

		expect(request.schedule.intervalHours).toBe(8);
		await vi.waitFor(() =>
			expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe(`/definitions/${DEF_ID}`)
		);
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewDefinitionPage);
		await fillDefinition('Baseline', '');

		type(field('definition-name')!, '   ');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createDefinition).not.toHaveBeenCalled();
	});
});

describe('/definitions/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer that still commits', async () => {
		const first = await render(NewDefinitionPage);
		await fillDefinition('Baseline', 'Everything a laptop needs');

		expect(stashContext()).toBe('draft:definition:create');
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
		render(NewDefinitionPage);

		await vi.waitFor(() => expect(field('definition-name')?.value).toBe('Baseline'));
		expect(area('definition-description')?.value).toBe('Everything a laptop needs');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createDefinition).toHaveBeenCalledTimes(1));
		expect(api.createDefinition.mock.calls[0][0].name).toBe('Baseline');
	});
});

describe('/definitions — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {
		nav.url = new URL('https://control.test/definitions');
		render(DefinitionsPage);

		const create = await vi.waitFor(() => {
			const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
				(b) => b.textContent?.trim() === m.definitions_create()
			);
			expect(button).toBeTruthy();
			return button!;
		});
		create.click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/definitions/new'));
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
	});
});
