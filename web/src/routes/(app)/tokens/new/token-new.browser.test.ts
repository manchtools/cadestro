

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import * as m from '$lib/paraglide/messages';

const { USER_ID } = vi.hoisted(() => ({ USER_ID: '01JQZZ0000000000000000000A' }));

const api = vi.hoisted(() => ({
	createToken: vi.fn(),
	listTokens: vi.fn(),
	deleteToken: vi.fn(),
	setTokenDisabled: vi.fn()
}));
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/tokens/new') }));

vi.mock('$lib/sdk', async () => {
	const control = await import('$contract/cadestro/v1/control_pb');
	const common = await import('$contract/cadestro/v1/common_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		authStore: { user: { id: USER_ID }, hasPermission: () => true },
		configStore: { serverUrl: 'https://control.test' },
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn(async () => []),
		useDraft: <T>(_type: string, _id: string, initial: T) => {
			let data = initial;
			return {
				get data() {
					return data;
				},
				set data(next: T) {
					data = next;
				},
				update(partial: Partial<T>) {
					data = { ...data, ...partial };
				},
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
import NewTokenPage from './+page.svelte';
import TokensPage from '../+page.svelte';
import StageRail from '$lib/components/shell/stage-rail.svelte';
import {
	shell,
	resetShell,
	setShellPath,
	stashContext,
	commitContext,
	pillMode
} from '$lib/shell/shell.svelte';

const ROUTE = '/tokens/new';
const TOKEN_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';

const CA_PIN = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';

beforeEach(() => {
	vi.clearAllMocks();
	resetShell();
	setShellPath(ROUTE);
	nav.url = new URL('https://control.test/tokens/new');

	api.createToken.mockResolvedValue({
		token: { id: TOKEN_ID, name: 'Fleet rollout', value: 'TOK-SECRET' },
		caFingerprintPin: CA_PIN
	});
	api.listTokens.mockResolvedValue({ tokens: [], nextPageToken: '' });
});

const field = (id: string) => document.querySelector<HTMLInputElement>(`#${id}`);

function type(input: HTMLInputElement, value: string) {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function fillToken(name: string, days: string) {
	await vi.waitFor(() => expect(field('token-name')).toBeTruthy());
	type(field('token-name')!, name);
	type(field('token-expires')!, days);
	await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
}

describe('/tokens/new — the commit is the pill\'s', () => {
	it('carries no Save button of its own', async () => {
		render(NewTokenPage);
		await fillToken('Fleet rollout', '7');

		expect(
			[...document.querySelectorAll('button')].some(
				(b) => b.textContent?.trim() === m.tokens_create()
			)
		).toBe(false);

		expect(shell.pill.context?.commitLabel).toBe(m.common_create());

		expect(shell.pill.context?.route).toBe(ROUTE);
	});

	it('creates the token with the exact arguments the dialog sent', async () => {
		render(NewTokenPage);
		await fillToken('Fleet rollout', '7');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createToken).toHaveBeenCalledTimes(1));
		expect(api.createToken.mock.calls[0][0]).toBe('Fleet rollout');
		expect(api.createToken.mock.calls[0][1]).toBe(0);
		expect(api.createToken.mock.calls[0][2]).toBeInstanceOf(Date);
	});

	it('sends an expiry date when the operator asks for one', async () => {
		render(NewTokenPage);
		await fillToken('Two week token', '14');

		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createToken).toHaveBeenCalledTimes(1));
		const expiresAt = api.createToken.mock.calls[0][2] as Date;
		expect(expiresAt).toBeInstanceOf(Date);
		expect(Math.round((expiresAt.getTime() - Date.now()) / 86_400_000)).toBe(14);
	});

	it('shows the bearer secret on the route, because navigating away would destroy it', async () => {
		render(NewTokenPage);
		await fillToken('Fleet rollout', '7');
		expect(commitContext()).toBe(true);

		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="token-secret"]')?.textContent).toBe(
				'TOK-SECRET'
			)
		);

		await vi.waitFor(() => expect(pillMode()).toBe('nav'));
		expect(vi.mocked(goto)).not.toHaveBeenCalled();
	});

	it('shows the CA fingerprint pin beside the secret, copyable, and in the install command', async () => {
		render(NewTokenPage);
		await fillToken('Fleet rollout', '7');
		expect(commitContext()).toBe(true);

		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="token-ca-pin"]')?.textContent).toBe(CA_PIN)
		);

		const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
		try {
			const copyPin = document.querySelector<HTMLButtonElement>(
				`button[aria-label="${m.tokens_copy_ca_pin()}"]`
			);
			expect(copyPin).toBeTruthy();
			copyPin!.click();
			expect(writeText).toHaveBeenCalledWith(CA_PIN);
		} finally {
			writeText.mockRestore();
		}

		const command = [...document.querySelectorAll('pre')].map((p) => p.textContent).join('\n');
		expect(command).toContain('-t TOK-SECRET');
		expect(command).toContain(`-p ${CA_PIN}`);
	});

	it('blocks the commit at the STORE while the name is missing', async () => {
		render(NewTokenPage);
		await vi.waitFor(() => expect(field('token-name')).toBeTruthy());
		type(field('token-name')!, 'temp');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true));

		type(field('token-name')!, '');
		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(false));
		expect(commitContext()).toBe(false);
		expect(api.createToken).not.toHaveBeenCalled();
	});
});

describe('/tokens/new — the third exit: stash, walk away, restore', () => {
	it('navigates home and rebuilds a buffer that still commits', async () => {
		const first = await render(NewTokenPage);
		await fillToken('Fleet rollout', '14');

		const draftId = stashContext();
		expect(draftId).toBe('draft:token:create');
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
		render(NewTokenPage);

		await vi.waitFor(() => expect(field('token-name')?.value).toBe('Fleet rollout'));
		expect(field('token-expires')?.value).toBe('14');
		expect(shell.drafts).toHaveLength(0);

		await vi.waitFor(() => expect(shell.pill.context?.valid).toBe(true), { timeout: 3000 });
		expect(commitContext()).toBe(true);
		await vi.waitFor(() => expect(api.createToken).toHaveBeenCalledTimes(1));
		expect(api.createToken.mock.calls[0][0]).toBe('Fleet rollout');
	});
});

describe('/tokens — the list page hands creation to the route', () => {
	it('navigates instead of opening a dialog', async () => {
		nav.url = new URL('https://control.test/tokens');
		render(TokensPage);
		await vi.waitFor(() => expect(api.listTokens).toHaveBeenCalled(), { timeout: 3000 });

		const create = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
			(b) => b.textContent?.trim() === m.tokens_create()
		);
		expect(create).toBeTruthy();
		create!.click();

		await vi.waitFor(() => expect(vi.mocked(goto).mock.calls[0]?.[0]).toBe('/tokens/new'));
		expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
	});
});
