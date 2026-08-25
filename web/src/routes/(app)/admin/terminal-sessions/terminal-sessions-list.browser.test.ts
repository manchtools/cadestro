// Conversion contract for the active-terminal-sessions list page.
//
// ListActiveTerminalSessions has no Search scope, so the RPC returns the set and
// the page's own matching, sorting and paging decide what an operator sees.
// These tests pin what the row-grammar conversion could quietly lose: the list
// is rows and never a table, the only navigation off a row is its device link,
// the destructive Terminate stays outside that link, and terminating still sends
// the session id together with the operator's reason.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { TerminalSessionInfoSchema } from '$contract/cadestro/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const SESSION_A = '01JQZZ4A7K3M9P2Q6R8T1V0W5X';
const SESSION_B = '01JQZZ5B8N4P0R3S7T9V2W1X6Y';
const DEVICE_A = '01JQZZ6C9P5Q1S4T8V0W3X2Y7Z';
const DEVICE_B = '01JQZZ7D0Q6R2T5V9W1X4Y3Z8A';
const USER_ID = '01JQZZ8E1R7S3V6W0X2Y5Z4A9B';

const api = vi.hoisted(() => ({
	listActiveTerminalSessions: vi.fn(),
	terminateTerminalSession: vi.fn()
}));
// Mutable so each test can mount the page "at" a different deep link.
const nav = vi.hoisted(() => ({ url: new URL('https://control.test/admin/terminal-sessions') }));

// Only the client is faked; the generated protobuf re-exports stay real.
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
		formatDuration: () => '—',
		fetchAllPages: vi.fn(async () => [])
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

// URL writes go through SvelteKit's shallow-routing API, which needs a live
// router; the history behaviour itself is url-state's contract, not this test's.
vi.mock('$app/navigation', () => ({
	pushState: vi.fn(),
	replaceState: vi.fn(),
	goto: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

import TerminalSessionsPage from './+page.svelte';

const sessions = [
	create(TerminalSessionInfoSchema, {
		sessionId: SESSION_A,
		userId: USER_ID,
		userEmail: 'operator@example.test',
		deviceId: { value: DEVICE_A },
		deviceHostname: 'ws-alpha',
		ttyUser: 'cadestro-shell-1',
		startedAt: timestampFromMs(Date.UTC(2026, 6, 3)),
		lastActivityAt: timestampFromMs(Date.UTC(2026, 6, 4))
	}),
	create(TerminalSessionInfoSchema, {
		sessionId: SESSION_B,
		userId: USER_ID,
		// No email and no hostname: the row must fall back to the ULIDs.
		deviceId: { value: DEVICE_B },
		ttyUser: 'cadestro-shell-2',
		startedAt: timestampFromMs(Date.UTC(2026, 6, 1)),
		lastActivityAt: timestampFromMs(Date.UTC(2026, 6, 2))
	})
];

beforeEach(() => {
	document.body.innerHTML = '';
	nav.url = new URL('https://control.test/admin/terminal-sessions');
	api.listActiveTerminalSessions.mockReset();
	api.terminateTerminalSession.mockReset();
	api.listActiveTerminalSessions.mockResolvedValue({ sessions, nextPageToken: '' });
	api.terminateTerminalSession.mockResolvedValue({});
});

async function mountAt(query: string) {
	nav.url = new URL(`https://control.test/admin/terminal-sessions${query}`);
	render(TerminalSessionsPage);
	await vi.waitFor(() => expect(api.listActiveTerminalSessions).toHaveBeenCalled(), {
		timeout: 3000
	});
}

/** The dialog is portalled, so it is addressed by its content slot, not by a
 *  role name the row's own Terminate button also answers to. */
function dialogButton(label: string): HTMLElement {
	const dialog = document.querySelector('[data-slot="alert-dialog-content"]');
	if (!dialog) throw new Error('the terminate dialog never opened');
	const button = [...dialog.querySelectorAll('button')].find(
		(b) => b.textContent?.trim() === label
	);
	if (!button) throw new Error(`no dialog button named ${label}`);
	return button;
}

describe('terminal sessions list — the list RPC feeds a client-side row list', () => {
	it('asks for the active sessions and renders the evidence each row carries', async () => {
		await mountAt('');

		expect(api.listActiveTerminalSessions).toHaveBeenCalledWith(100);
		await expect.element(browser.getByText(SESSION_A)).toBeVisible();
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();
		await expect.element(browser.getByText('operator@example.test')).toBeVisible();
		await expect.element(browser.getByText('cadestro-shell-1')).toBeVisible();
	});

	it('falls back to the ULIDs when the session carries no email or hostname', async () => {
		await mountAt('');

		await expect.element(browser.getByText(DEVICE_B)).toBeVisible();
		await expect.element(browser.getByText(USER_ID).first()).toBeVisible();
	});

	it('renders the list in the row grammar — never a table', async () => {
		await mountAt('');
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();

		expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull();
		expect(document.querySelectorAll('table').length).toBe(0);
	});

	// The column headers that carried these keys are gone; the same keys now ride
	// RowList's sort bar, and the ?sort / ?sortDir contract is unchanged.
	it('opens newest-first and re-sorts from the row list sort bar', async () => {
		await mountAt('');
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();

		const order = () =>
			[...document.querySelectorAll('[data-testid="row-list-row"]')].map((r) =>
				r.getAttribute('data-row-key')
			);
		await vi.waitFor(() => expect(order()).toEqual([SESSION_A, SESSION_B]), { timeout: 3000 });

		const userSort = [
			...document.querySelectorAll<HTMLButtonElement>('[data-testid="row-list-sort"] button')
		].find((b) => b.textContent?.trim().startsWith(m.terminal_sessions_user()));
		expect(userSort).toBeDefined();
		userSort!.click();

		// Ascending by user: the session with no email sorts on its ULID, which
		// precedes the resolved address.
		await vi.waitFor(() => expect(order()).toEqual([SESSION_B, SESSION_A]), { timeout: 3000 });
	});

	it('matches the query against the hostname, the email and the ULIDs', async () => {
		await mountAt('?query=ws-alpha');

		await vi.waitFor(
			() => expect(document.querySelectorAll('[data-testid="row-list-row"]')).toHaveLength(1),
			{ timeout: 3000 }
		);
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();
	});
});

describe('terminal sessions list — the row links to its device and nothing else', () => {
	it('points each device link at that device page', async () => {
		await mountAt('');
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();

		const links = [
			...document.querySelectorAll<HTMLAnchorElement>('[data-testid="terminal-session-device-link"]')
		].map((a) => a.getAttribute('href'));
		expect(links).toEqual([`/devices/${DEVICE_A}`, `/devices/${DEVICE_B}`]);
		// A session has no page of its own, so the row body is never one link.
		expect(document.querySelectorAll('[data-testid="row-list-link"]').length).toBe(0);
	});

	// A button may never nest inside an anchor: the destructive control lives in
	// RowList's rowEnd slot, outside the row body that carries the device link.
	it('keeps Terminate outside the device link', async () => {
		await mountAt('');
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();

		const terminate = [...document.querySelectorAll('button')].filter(
			(b) => b.textContent?.trim() === m.terminal_sessions_terminate()
		);
		expect(terminate).toHaveLength(sessions.length);
		for (const button of terminate) {
			expect(button.closest('a')).toBeNull();
		}
	});
});

describe('terminal sessions list — terminating a session', () => {
	it('sends the session id with the operator reason and drops the row', async () => {
		await mountAt('');
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();

		await browser.getByRole('button', { name: m.terminal_sessions_terminate() }).first().click();

		const reason = document.getElementById('terminate-reason');
		expect(reason).not.toBeNull();
		await browser.elementLocator(reason!).fill('incident 4711');

		await browser.elementLocator(dialogButton(m.terminal_sessions_terminate())).click();

		await vi.waitFor(
			() =>
				expect(api.terminateTerminalSession).toHaveBeenCalledWith(SESSION_A, 'incident 4711'),
			{ timeout: 3000 }
		);
		// The session is gone server-side, so the row goes without a refetch.
		await vi.waitFor(
			() => expect(document.querySelectorAll('[data-testid="row-list-row"]')).toHaveLength(1),
			{ timeout: 3000 }
		);
		expect(api.listActiveTerminalSessions).toHaveBeenCalledTimes(1);
	});

	it('keeps the row when the terminate RPC fails', async () => {
		api.terminateTerminalSession.mockRejectedValue(new Error('permission denied'));
		await mountAt('');
		await expect.element(browser.getByText('ws-alpha')).toBeVisible();

		await browser.getByRole('button', { name: m.terminal_sessions_terminate() }).first().click();
		await browser.elementLocator(dialogButton(m.terminal_sessions_terminate())).click();

		await vi.waitFor(() => expect(api.terminateTerminalSession).toHaveBeenCalled(), {
			timeout: 3000
		});
		expect(document.querySelectorAll('[data-testid="row-list-row"]')).toHaveLength(2);
	});
});

describe('terminal sessions list — empty states', () => {
	it('distinguishes "no sessions" from "nothing matched"', async () => {
		api.listActiveTerminalSessions.mockResolvedValue({ sessions: [], nextPageToken: '' });
		await mountAt('');
		await expect.element(browser.getByText(m.terminal_sessions_empty_hint())).toBeVisible();

		api.listActiveTerminalSessions.mockResolvedValue({ sessions, nextPageToken: '' });
		await mountAt('?query=no-such-host');
		await expect.element(browser.getByText(m.common_try_different_search())).toBeVisible();
	});
});
