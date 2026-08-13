// The device level's status column, exercised through the REAL row component
// against a mocked search RPC.
//
// The fleet model classifies a device into FOUR mutually exclusive buckets —
// ok (reachable and compliant), warn (compliance drift), crit (offline) and
// idle (never seen). The row's status cell must NAME the bucket its tile was
// painted from. It used to render `tone === 'ok' ? online : offline`, which
// collapsed three of those buckets into the single word "offline": a drifting
// device that control is talking to right now claimed to be unreachable, and a
// device control has never heard from claimed the same. These tests pin the
// label against the tone, so re-collapsing the ternary fails here.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { ComplianceStatus } from '$sdk/powermanage/v1/common_pb';
import { SearchResultSchema } from '$sdk/powermanage/v1/control_pb';
import * as m from '$lib/paraglide/messages';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/devices?zoom=device'),
	search: vi.fn(),
	goto: vi.fn(),
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
	goto: mocks.goto,
	pushState: mocks.pushState,
	replaceState: mocks.replaceState,
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

// Only the client is faked; the generated protobuf re-exports stay real, so the
// component classifies with the production DeviceStatus / ComplianceStatus.
vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			search: mocks.search,
			deleteDevice: vi.fn(),
			assignDevice: vi.fn(),
			listUsers: vi.fn().mockResolvedValue({ users: [] }),
			listUserGroups: vi.fn().mockResolvedValue({ groups: [] })
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-08-01 09:00'
	};
});

import DeviceLevel from './device-level.svelte';
import { resetFleetSelection } from './fleet-selection.svelte';

const nowSec = () => Math.floor(Date.now() / 1000);

/** A search hit shaped like the control index's device document. */
function hit(o: {
	id: string;
	hostname: string;
	/** seconds ago; null = never seen (no last_seen_at in the document) */
	seen: number | null;
	compliance?: ComplianceStatus;
}) {
	const fields: Record<string, string> = {
		hostname: o.hostname,
		agent_version: '1.2.3',
		compliance_status: String(o.compliance ?? ComplianceStatus.COMPLIANT)
	};
	if (o.seen !== null) fields['last_seen_at'] = String(nowSec() - o.seen);
	return create(SearchResultSchema, { id: o.id, name: o.hostname, fields });
}

/** The status cell for one row, addressed by the tone it was painted with. */
function statusCells() {
	return Array.from(document.querySelectorAll<HTMLElement>('[data-testid="device-status"]'));
}

beforeEach(() => {
	document.body.innerHTML = '';
	mocks.url = new URL('http://localhost/devices?zoom=device');
	mocks.search.mockReset();
	mocks.goto.mockReset();
	mocks.pushState.mockReset();
	mocks.replaceState.mockReset();
	resetFleetSelection();
});

describe('device level — the status label names the bucket the tile was painted from', () => {
	it('labels drift as drift and never-seen as never seen, not as offline', async () => {
		mocks.search.mockResolvedValue({
			results: [
				// online AND non-compliant => warn (drift), not offline
				hit({ id: 'd-warn', hostname: 'drift-01', seen: 30, compliance: ComplianceStatus.NON_COMPLIANT }),
				// no last_seen_at at all => idle (never seen), not offline
				hit({ id: 'd-idle', hostname: 'never-01', seen: null })
			],
			totalCount: 2
		});

		render(DeviceLevel, { surfaceId: 'devices' });

		await vi.waitFor(() => expect(statusCells().length).toBe(2), { timeout: 3000 });

		const byTone = Object.fromEntries(statusCells().map((c) => [c.dataset.tone, c]));
		// The buckets themselves must survive: a collapsed classifier would not
		// even produce these two tones.
		expect(Object.keys(byTone).sort()).toEqual(['idle', 'warn']);

		expect(byTone.warn.textContent).toContain(m.fleet_status_drift());
		expect(byTone.warn.textContent).not.toContain(m.devices_status_offline());

		expect(byTone.idle.textContent).toContain(m.fleet_tile_idle());
		expect(byTone.idle.textContent).not.toContain(m.devices_status_offline());
	});

	it('still labels a reachable device online and an unreachable one offline', async () => {
		mocks.search.mockResolvedValue({
			results: [
				hit({ id: 'd-ok', hostname: 'api-01', seen: 30 }),
				// last seen well outside the server's five-minute online window
				hit({ id: 'd-crit', hostname: 'api-02', seen: 60 * 60 })
			],
			totalCount: 2
		});

		render(DeviceLevel, { surfaceId: 'devices' });

		await vi.waitFor(() => expect(statusCells().length).toBe(2), { timeout: 3000 });

		const byTone = Object.fromEntries(statusCells().map((c) => [c.dataset.tone, c]));
		expect(Object.keys(byTone).sort()).toEqual(['crit', 'ok']);
		expect(byTone.ok.textContent).toContain(m.devices_status_online());
		expect(byTone.crit.textContent).toContain(m.devices_status_offline());
	});
});
