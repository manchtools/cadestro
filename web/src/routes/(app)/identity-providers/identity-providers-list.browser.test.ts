

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { IdentityProviderSchema } from '$contract/cadestro/v1/control_pb';
import { IdentityProviderType } from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';

const ENABLED_ID = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const DISABLED_ID = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';

const api = vi.hoisted(() => ({
	listIdentityProviders: vi.fn(),
	deleteIdentityProvider: vi.fn(),
	createIdentityProvider: vi.fn()
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

const nav = vi.hoisted(() => ({ url: new URL('https://control.test/identity-providers') }));

vi.mock('svelte-sonner', () => ({ toast }));

vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: api,
		formatTimestamp: () => '2026-08-01',
		formatTimestampDateTime: () => '2026-08-01 09:00',
		fetchAllPages: vi.fn()
	};
});

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return nav.url;
		}
	}
}));

vi.mock('$app/paths', () => ({ base: '', assets: '' }));

vi.mock('$app/navigation', () => ({
	pushState: vi.fn(),
	replaceState: vi.fn(),
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import IdentityProvidersPage from './+page.svelte';

const providers = [
	create(IdentityProviderSchema, {
		id: { value: ENABLED_ID },
		name: 'Corporate Entra',
		slug: 'entra',
		providerType: IdentityProviderType.OIDC,
		enabled: true,
		clientId: { value: 'client-abc' },
		issuerUrl: 'https://login.example.test',
		createdAt: timestampFromMs(Date.UTC(2026, 6, 1))
	}),
	create(IdentityProviderSchema, {
		id: { value: DISABLED_ID },
		name: 'Legacy Okta',
		slug: 'okta',
		providerType: IdentityProviderType.OIDC,
		enabled: false,
		clientId: { value: 'client-def' },
		issuerUrl: 'https://okta.example.test',
		createdAt: timestampFromMs(Date.UTC(2026, 6, 2))
	})
];

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	nav.url = new URL('https://control.test/identity-providers');
	api.listIdentityProviders.mockResolvedValue({ providers });
});

function rowKeys(): string[] {
	return [...document.querySelectorAll<HTMLElement>('[data-testid="row-list-row"]')].map(
		(el) => el.getAttribute('data-row-key') ?? ''
	);
}

function clickSort(label: string) {
	const button = [
		...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
	].find((b) => b.textContent?.trim().startsWith(label));
	if (!button) throw new Error(`no sort control named ${label}`);
	button.click();
}

async function mountAt(query: string) {
	nav.url = new URL(`https://control.test/identity-providers${query}`);
	render(IdentityProvidersPage);
	await vi.waitFor(() => expect(api.listIdentityProviders).toHaveBeenCalled(), { timeout: 3000 });
}

describe('identity-providers list — the row grammar', () => {
	it('renders the list as dense rows, never a table', async () => {
		await mountAt('');
		await vi.waitFor(() => expect(rowKeys()).toEqual([ENABLED_ID, DISABLED_ID]), {
			timeout: 3000
		});

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	it('makes each row a link to its provider detail', async () => {
		await mountAt('');
		await vi.waitFor(() => expect(rowKeys()).toHaveLength(2), { timeout: 3000 });

		const links = [
			...document.querySelectorAll<HTMLAnchorElement>('[data-testid="row-list-link"]')
		].map((a) => a.getAttribute('href'));
		expect(links).toEqual([
			`/identity-providers/${ENABLED_ID}`,
			`/identity-providers/${DISABLED_ID}`
		]);
	});

	it('keeps the ULID, the slug, the OIDC type and the enabled state on the row', async () => {
		await mountAt('');

		await expect.element(browser.getByText('Corporate Entra')).toBeVisible();
		await expect.element(browser.getByText(ENABLED_ID)).toBeVisible();

		await expect.element(browser.getByText('entra', { exact: true })).toBeVisible();
		await expect.element(browser.getByText('okta', { exact: true })).toBeVisible();
		await expect.element(browser.getByText(m.idp_enabled(), { exact: true })).toBeVisible();
		await expect.element(browser.getByText(m.idp_disabled(), { exact: true })).toBeVisible();
		expect(
			[...document.querySelectorAll('[data-testid="fleet-chip"]')].filter(
				(c) => c.textContent?.trim() === m.idp_type_oidc()
			)
		).toHaveLength(2);
	});

	it('re-sorts by slug from the sort bar', async () => {
		await mountAt('');

		await vi.waitFor(() => expect(rowKeys()).toEqual([ENABLED_ID, DISABLED_ID]), {
			timeout: 3000
		});

		clickSort(m.idp_field_slug());

		clickSort(m.idp_field_slug());
		await vi.waitFor(() => expect(rowKeys()).toEqual([DISABLED_ID, ENABLED_ID]), {
			timeout: 3000
		});
	});

	it('matches the search box query against the provider name', async () => {
		await mountAt('?query=okta');

		await vi.waitFor(() => expect(rowKeys()).toEqual([DISABLED_ID]), { timeout: 3000 });
	});
});

describe('identity-providers list — secret hygiene', () => {
	it('never renders a client secret for a listed provider', async () => {

		await mountAt('');
		await expect.element(browser.getByText('Corporate Entra')).toBeVisible();

		expect(document.body.textContent).not.toContain('client-abc');
		expect(document.body.textContent).not.toContain('client-def');
	});
});

describe('identity-providers list — empty states', () => {
	it('distinguishes "no providers yet" from "nothing matched"', async () => {
		api.listIdentityProviders.mockResolvedValue({ providers: [] });
		await mountAt('');
		await expect.element(browser.getByText(m.idp_empty_description())).toBeVisible();

		document.body.innerHTML = '';
		api.listIdentityProviders.mockResolvedValue({ providers });
		await mountAt('?query=nothing-matches-this');
		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});
