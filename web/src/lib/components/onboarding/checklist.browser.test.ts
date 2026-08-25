

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ConnectError, Code } from '@connectrpc/connect';

const mocks = vi.hoisted(() => ({
	listDevices: vi.fn(),
	listTokens: vi.fn(),
	listActions: vi.fn(),
	listAssignments: vi.fn(),
	listUsers: vi.fn(),
	listIdentityProviders: vi.fn()
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));
vi.mock('$lib/sdk', () => ({ apiClient: mocks }));

import Checklist from './getting-started-checklist.svelte';
import { resetOnboarding } from '$lib/onboarding';

const denied = () => new ConnectError('permission denied', Code.PermissionDenied);

function happyFleet() {
	mocks.listDevices.mockResolvedValue({ devices: [{ id: 'd1' }], totalCount: 1, nextPageToken: '' });
	mocks.listTokens.mockResolvedValue({ tokens: [] });
	mocks.listActions.mockResolvedValue({ actions: [{ id: 'a1' }] });
	mocks.listAssignments.mockResolvedValue({ assignments: [] });
	mocks.listUsers.mockResolvedValue({ users: [{ id: 'u1' }, { id: 'u2' }] });
	mocks.listIdentityProviders.mockResolvedValue({ providers: [] });
}

const rows = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="onboarding-checklist-row"]'));
const statuses = () => Object.fromEntries(rows().map((r) => [r.dataset.check, r.dataset.status]));
const progress = () =>
	document.querySelector('[data-testid="onboarding-checklist-progress"]')?.textContent?.trim() ?? '';

let mounted: { unmount?: () => void } | null = null;

beforeEach(() => {
	document.body.innerHTML = '';
	localStorage.clear();
	resetOnboarding();
	for (const fn of Object.values(mocks)) fn.mockReset();
	happyFleet();
});

afterEach(() => {
	mounted?.unmount?.();
	mounted = null;
});

function mount() {
	mounted = render(Checklist) as { unmount?: () => void };
}

describe('rows are the RPC answer', () => {
	it('marks each row from its own read and counts only answered rows', async () => {
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		expect(statuses()).toEqual({
			device: 'done',
			token: 'todo',
			action: 'done',
			assignment: 'todo',
			people: 'done'
		});
		expect(progress()).toBe('3 of 5 steps completed');

		expect(mocks.listDevices).toHaveBeenCalledWith(1);
		expect(mocks.listUsers).toHaveBeenCalledWith(2);
	});

	it('links every row at the surface that does the work', async () => {
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		expect(Object.fromEntries(rows().map((r) => [r.dataset.check, r.getAttribute('href')]))).toEqual({
			device: '/devices',
			token: '/tokens',
			action: '/actions',
			assignment: '/assign',
			people: '/users'
		});
	});

	it('reports an empty fleet as outstanding, never as done', async () => {
		mocks.listDevices.mockResolvedValue({ devices: [], totalCount: 0, nextPageToken: '' });
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		expect(statuses().device).toBe('todo');
		expect(progress()).toBe('2 of 5 steps completed');
	});

	it('counts an identity provider as bringing people in, without a second human', async () => {
		mocks.listUsers.mockResolvedValue({ users: [{ id: 'u1' }] });
		mocks.listIdentityProviders.mockResolvedValue({ providers: [{ id: 'idp' }] });
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		expect(statuses().people).toBe('done');
	});
});

describe('permissions and failures', () => {
	it('HIDES a row whose read is denied rather than showing it incomplete', async () => {
		mocks.listTokens.mockRejectedValue(denied());
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(4));

		expect(Object.keys(statuses())).not.toContain('token');
		expect(progress()).toBe('3 of 4 steps completed');
	});

	it('hides the people row only when EVERY probe behind it is denied', async () => {
		mocks.listUsers.mockRejectedValue(denied());
		mocks.listIdentityProviders.mockResolvedValue({ providers: [] });
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		expect(statuses().people).toBe('todo');

		mounted?.unmount?.();
		document.body.innerHTML = '';
		mocks.listIdentityProviders.mockRejectedValue(denied());
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(4));
		expect(Object.keys(statuses())).not.toContain('people');
	});

	it('calls a failed read unknown, not undone, and leaves it out of the count', async () => {
		vi.spyOn(console, 'warn').mockImplementation(() => {});
		mocks.listAssignments.mockRejectedValue(new Error('network down'));
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		expect(statuses().assignment).toBe('unknown');

		expect(progress()).toBe('3 of 4 steps completed');
		vi.restoreAllMocks();
	});

	it('renders nothing at all when every check is denied', async () => {
		for (const fn of Object.values(mocks)) fn.mockRejectedValue(denied());
		mount();
		await vi.waitFor(() => expect(document.querySelector('[data-testid="onboarding-checklist-loading"]')).toBeNull());
		expect(document.querySelector('[data-testid="onboarding-checklist"]')).toBeNull();
	});
});

describe('dismissal', () => {
	it('stays dismissed once the operator closes it', async () => {
		mount();
		await vi.waitFor(() => expect(rows().length).toBe(5));

		document.querySelector<HTMLButtonElement>('[data-testid="onboarding-checklist-dismiss"]')!.click();

		await vi.waitFor(() => expect(document.querySelector('[data-testid="onboarding-checklist"]')).toBeNull());
	});
});
