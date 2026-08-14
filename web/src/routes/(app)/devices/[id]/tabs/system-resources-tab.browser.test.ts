// Reveal-per-entry contract for the device system-resources tab.
//
// The list RPCs return metadata only; plaintext exists solely behind
// RevealLpsPassword / RevealLuksKey, and every one of those calls is an
// audited sensitive read on the server. These tests pin the properties that
// make that audit trail honest: nothing is revealed on load, a reveal asks
// for exactly one entry's ULID, and hiding drops the plaintext so the next
// reveal has to go back to the server instead of replaying a cached value.
//
// The LUKS token dialog is pinned here too, from both sides. The token must be
// ON SCREEN — an operator on SSH has no URI handler and the advertised command
// only prompts for it, so a dialog without the token is a dead end. And the
// token must stay OUT of the command string: argv is world-readable through
// /proc/<pid>/cmdline, which is why the server stopped advertising it there.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import {
	CreateLuksTokenResponseSchema,
	LpsPasswordSchema,
	LuksKeySchema
} from '$contract/cadestro/v1/control_pb';
import { RotationReason, LuksRevocationStatus } from '$contract/cadestro/v1/common_pb';
import * as m from '$lib/paraglide/messages';

const DEVICE_ID = '01JQZZDEVICE00000000000000';
const ALICE_ID = '01JQZZLPSALICE000000000000';
const BOB_ID = '01JQZZLPSBOB00000000000000';
const LUKS_ID = '01JQZZLUKSCURRENT000000000';

const LUKS_ACTION_ID = '01JQZZACTIONLUKS000000000B';

const ALICE_PASSWORD = 'alice-plaintext-9f2a';
const BOB_PASSWORD = 'bob-plaintext-4c1d';
const LUKS_PASSPHRASE = 'luks-plaintext-7b3e';
const LUKS_TOKEN = 'c0ffee00-token-0001';
const SECOND_LUKS_TOKEN = 'c0ffee00-token-0002';

// What the server actually advertises: no token on argv, no sudo.
const LUKS_CLI_COMMAND = 'cadestrod luks set-passphrase';

const api = vi.hoisted(() => ({
	listLpsPasswords: vi.fn(),
	revealLpsPassword: vi.fn(),
	listLuksKeys: vi.fn(),
	revealLuksKey: vi.fn(),
	createLuksToken: vi.fn(),
	revokeLuksDeviceKey: vi.fn()
}));

// Only the two values the tab imports; the rest of `$lib/sdk` is types.
vi.mock('$lib/sdk', () => ({
	apiClient: api,
	formatTimestampDateTime: () => '2026-08-01 09:00'
}));

// `vi.mock` is hoisted above this import, so the tab binds to the stubbed client.
import SystemResourcesTab from './system-resources-tab.svelte';

const lpsCurrent = [
	create(LpsPasswordSchema, {
		id: ALICE_ID,
		deviceId: DEVICE_ID,
		actionId: '01JQZZACTIONLPS0000000000A',
		actionName: 'Workstation LPS',
		username: 'alice',
		rotationReason: RotationReason.SCHEDULED
	}),
	create(LpsPasswordSchema, {
		id: BOB_ID,
		deviceId: DEVICE_ID,
		actionId: '01JQZZACTIONLPS0000000000A',
		actionName: 'Workstation LPS',
		username: 'bob',
		rotationReason: RotationReason.INITIAL
	})
];

const luksCurrent = [
	create(LuksKeySchema, {
		id: LUKS_ID,
		deviceId: DEVICE_ID,
		actionId: LUKS_ACTION_ID,
		actionName: 'Laptop LUKS',
		devicePath: '/dev/sda3',
		rotationReason: RotationReason.SCHEDULED,
		revocationStatus: LuksRevocationStatus.NONE
	})
];

const plaintextByEntry: Record<string, string> = {
	[ALICE_ID]: ALICE_PASSWORD,
	[BOB_ID]: BOB_PASSWORD
};

/** A CreateLuksToken response built from the generated schema, so a field
 *  rename in the contract fails these tests instead of silently passing. */
function tokenResponse(token: string) {
	return create(CreateLuksTokenResponseSchema, {
		token,
		uri: `cadestro://luks/set-passphrase?token=${token}`,
		cliCommand: LUKS_CLI_COMMAND
	});
}

const clipboardWrite = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();
	api.listLpsPasswords.mockResolvedValue({ current: lpsCurrent, history: [] });
	api.listLuksKeys.mockResolvedValue({ current: luksCurrent, history: [] });
	api.revealLpsPassword.mockImplementation(async (id: string) => ({
		password: plaintextByEntry[id] ?? 'unknown-entry'
	}));
	api.revealLuksKey.mockResolvedValue({ passphrase: LUKS_PASSPHRASE });
	api.createLuksToken.mockResolvedValue(tokenResponse(LUKS_TOKEN));
	// Headless Chromium gates the real clipboard behind a permission prompt;
	// what matters here is the exact string each button hands over.
	clipboardWrite.mockResolvedValue(undefined);
	Object.defineProperty(navigator, 'clipboard', {
		value: { writeText: clipboardWrite },
		configurable: true
	});
});

/** Locator for one row's secret cell, addressed by its secret-entry ULID. */
function secretCell(entryId: string) {
	const el = document.querySelector(`[data-testid="secret-cell"][data-entry-id="${entryId}"]`);
	if (!el) throw new Error(`no secret cell rendered for entry ${entryId}`);
	return page.elementLocator(el);
}

async function mountTab() {
	render(SystemResourcesTab, { deviceId: DEVICE_ID });
	// Wait for the metadata load to paint before touching rows.
	await expect.element(page.getByText('alice')).toBeVisible();
}

/** Issue a token from the LUKS row and wait for the dialog to paint. */
async function openLuksTokenDialog() {
	await page.getByRole('button', { name: m.luks_set_passphrase(), exact: true }).click();
	await expect.element(page.getByTestId('luks-token')).toBeVisible();
}

describe('system resources tab — secret metadata load', () => {
	it('loads metadata for the device and reveals nothing', async () => {
		await mountTab();

		expect(api.listLpsPasswords).toHaveBeenCalledWith(DEVICE_ID);
		expect(api.listLuksKeys).toHaveBeenCalledWith(DEVICE_ID);
		expect(api.revealLpsPassword).not.toHaveBeenCalled();
		expect(api.revealLuksKey).not.toHaveBeenCalled();

		for (const secret of [ALICE_PASSWORD, BOB_PASSWORD, LUKS_PASSPHRASE]) {
			expect(document.body.textContent).not.toContain(secret);
		}
		await expect.element(secretCell(ALICE_ID)).toHaveTextContent('••••••••••••');
		await expect.element(secretCell(LUKS_ID)).toHaveTextContent('••••••••••••');
	});
});

describe('system resources tab — reveal per entry', () => {
	it('reveals exactly the clicked row and leaves its neighbours masked', async () => {
		await mountTab();

		const alice = secretCell(ALICE_ID);
		await expect
			.element(alice.getByRole('button', { name: m.lps_passwords_copy() }))
			.toBeDisabled();

		await alice.getByRole('button', { name: m.lps_passwords_reveal() }).click();

		await expect.element(alice).toHaveTextContent(ALICE_PASSWORD);
		expect(api.revealLpsPassword).toHaveBeenCalledTimes(1);
		expect(api.revealLpsPassword).toHaveBeenCalledWith(ALICE_ID);
		expect(api.revealLuksKey).not.toHaveBeenCalled();

		// The other rows never asked the server, so they stay masked.
		expect(document.body.textContent).not.toContain(BOB_PASSWORD);
		expect(document.body.textContent).not.toContain(LUKS_PASSPHRASE);

		// Copy only becomes usable once this row holds plaintext.
		await expect.element(alice.getByRole('button', { name: m.lps_passwords_copy() })).toBeEnabled();
	});

	it('reveals a LUKS passphrase with that key entry ULID', async () => {
		await mountTab();

		const luks = secretCell(LUKS_ID);
		await luks.getByRole('button', { name: m.luks_keys_reveal() }).click();

		await expect.element(luks).toHaveTextContent(LUKS_PASSPHRASE);
		expect(api.revealLuksKey).toHaveBeenCalledTimes(1);
		expect(api.revealLuksKey).toHaveBeenCalledWith(LUKS_ID);
	});

	it('hiding drops the plaintext and re-revealing audits a second read', async () => {
		await mountTab();

		const alice = secretCell(ALICE_ID);
		await alice.getByRole('button', { name: m.lps_passwords_reveal() }).click();
		await expect.element(alice).toHaveTextContent(ALICE_PASSWORD);

		await alice.getByRole('button', { name: m.lps_passwords_hide() }).click();
		await expect.element(alice).toHaveTextContent('••••••••••••');
		expect(document.body.textContent).not.toContain(ALICE_PASSWORD);
		await expect
			.element(alice.getByRole('button', { name: m.lps_passwords_copy() }))
			.toBeDisabled();

		await alice.getByRole('button', { name: m.lps_passwords_reveal() }).click();
		await expect.element(alice).toHaveTextContent(ALICE_PASSWORD);
		expect(api.revealLpsPassword).toHaveBeenCalledTimes(2);
		expect(api.revealLpsPassword).toHaveBeenNthCalledWith(2, ALICE_ID);
	});

	it('disables the toggle while its reveal is in flight', async () => {
		let release!: (value: { password: string }) => void;
		api.revealLpsPassword.mockImplementation(
			() => new Promise<{ password: string }>((resolve) => (release = resolve))
		);
		await mountTab();

		const alice = secretCell(ALICE_ID);
		const toggle = alice.getByRole('button', { name: m.lps_passwords_reveal() });
		// The click resolves once dispatched; the reveal promise stays pending.
		await toggle.click();

		await expect.element(toggle).toBeDisabled();
		expect(api.revealLpsPassword).toHaveBeenCalledTimes(1);

		release({ password: ALICE_PASSWORD });
		await expect.element(alice).toHaveTextContent(ALICE_PASSWORD);
	});
});

describe('system resources tab — LUKS token dialog', () => {
	it('shows both routes: the URI for a desktop, the command AND the token for a terminal', async () => {
		await mountTab();
		await openLuksTokenDialog();

		expect(api.createLuksToken).toHaveBeenCalledTimes(1);
		expect(api.createLuksToken).toHaveBeenCalledWith(DEVICE_ID, LUKS_ACTION_ID);

		// Without this the SSH operator is stuck: the command prompts for a token
		// the dialog never gave them.
		await expect.element(page.getByTestId('luks-token')).toHaveTextContent(LUKS_TOKEN);

		// …and the token stays off the command line, where /proc would publish it.
		const command = page.getByTestId('luks-cli-command');
		await expect.element(command).toHaveTextContent(LUKS_CLI_COMMAND);
		expect(command.element().textContent).not.toContain(LUKS_TOKEN);

		await expect
			.element(page.getByRole('link', { name: m.luks_set_passphrase() }))
			.toHaveAttribute('href', `cadestro://luks/set-passphrase?token=${LUKS_TOKEN}`);
	});

	it('copies the token and the command to the clipboard as separate values', async () => {
		await mountTab();
		await openLuksTokenDialog();

		await page.getByRole('button', { name: m.luks_copy_token() }).click();
		expect(clipboardWrite).toHaveBeenCalledWith(LUKS_TOKEN);

		await page.getByRole('button', { name: m.luks_copy_command() }).click();
		expect(clipboardWrite).toHaveBeenLastCalledWith(LUKS_CLI_COMMAND);
		expect(clipboardWrite).toHaveBeenCalledTimes(2);
	});

	it('drops the token when the dialog closes and never carries it into the next issuance', async () => {
		await mountTab();
		await openLuksTokenDialog();

		await page.getByRole('button', { name: m.common_done() }).click();
		await expect.element(page.getByTestId('luks-token')).not.toBeInTheDocument();
		expect(document.body.textContent).not.toContain(LUKS_TOKEN);

		// The holder that drives the dialog is the same one that holds the token,
		// so a second issuance can only paint if the first was really dropped —
		// and it paints its own token, never the retired one.
		api.createLuksToken.mockResolvedValue(tokenResponse(SECOND_LUKS_TOKEN));
		await openLuksTokenDialog();

		await expect.element(page.getByTestId('luks-token')).toHaveTextContent(SECOND_LUKS_TOKEN);
		expect(document.body.textContent).not.toContain(LUKS_TOKEN);
	});
});
