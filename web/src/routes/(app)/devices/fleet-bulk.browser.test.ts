// The fleet's two ambient/bulk behaviours, exercised through the REAL route
// component against mocked RPCs:
//
//   1. the resting pill explains the section beneath it — the caption carries
//      the SAME buckets the summary strip counts, and it leaves with the
//      surface instead of lingering over the next screen;
//   2. Reboot and Label write per device. What is load-bearing is that the
//      confirmation tells the truth about offline members, that EVERY selected
//      id is written (exact ids, one call each), and that one failing device
//      neither aborts the rest nor turns into a toast storm.
//
// Only `apiClient` and the toaster are faked; the generated protobuf enums, the
// shell store and the fleet selection store are the production modules.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browser } from 'vitest/browser';
import { create } from '@bufbuild/protobuf';
import { TimestampSchema } from '@bufbuild/protobuf/wkt';
import { ComplianceStatus, DeviceStatus } from '$contract/cadestro/v1/common_pb';
import { DeviceSchema, DeviceGroupSchema } from '$contract/cadestro/v1/control_pb';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/devices'),
	listDevices: vi.fn(),
	listDeviceGroups: vi.fn(),
	getDeviceGroup: vi.fn(),
	search: vi.fn(),
	rebootDevice: vi.fn(),
	setDeviceLabel: vi.fn(),
	goto: vi.fn(),
	pushState: vi.fn()
}));
const toaster = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

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
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));
vi.mock('svelte-sonner', () => ({ toast: toaster }));

vi.mock('$lib/sdk', async () => {
	const common = await import('$contract/cadestro/v1/common_pb');
	const control = await import('$contract/cadestro/v1/control_pb');
	const actions = await import('$contract/cadestro/v1/actions_pb');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			listDevices: mocks.listDevices,
			listDeviceGroups: mocks.listDeviceGroups,
			getDeviceGroup: mocks.getDeviceGroup,
			search: mocks.search,
			rebootDevice: mocks.rebootDevice,
			setDeviceLabel: mocks.setDeviceLabel,
			deleteDevice: vi.fn(),
			assignDevice: vi.fn(),
			listAvailableActions: vi.fn().mockResolvedValue([]),
			setUserSelection: vi.fn(),
			listUsers: vi.fn().mockResolvedValue({ users: [] }),
			listUserGroups: vi.fn().mockResolvedValue({ groups: [] })
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages: vi.fn().mockResolvedValue([])
	};
});

import DevicesPage from './+page.svelte';
import { shell, resetShell, runPillAction, pillMode, pillSubtext } from '$lib/shell/shell.svelte';
import { resetFleetSelection } from './fleet-selection.svelte';
import { setCarried } from '$lib/shell/carried-selection.svelte';

const MIN = 60;
const nowSec = () => Math.floor(Date.now() / 1000);

function device(o: {
	id: string;
	hostname: string;
	status?: DeviceStatus;
	compliance?: ComplianceStatus;
	/** minutes ago; null = never seen */
	seen?: number | null;
}) {
	return create(DeviceSchema, {
		id: { value: o.id },
		hostname: o.hostname,
		status: o.status ?? DeviceStatus.ONLINE,
		complianceStatus: o.compliance ?? ComplianceStatus.COMPLIANT,
		lastSeenAt:
			o.seen === null
				? undefined
				: create(TimestampSchema, { seconds: BigInt(nowSec() - (o.seen ?? 0) * MIN) })
	});
}

function respond(devices: ReturnType<typeof device>[], groups: Record<string, string[]> = {}) {
	mocks.listDevices.mockResolvedValue({ devices, nextPageToken: '', totalCount: devices.length });
	const protos = Object.keys(groups).map((id) => create(DeviceGroupSchema, { id: { value: id }, name: id }));
	mocks.listDeviceGroups.mockResolvedValue({
		groups: protos,
		nextPageToken: '',
		totalCount: protos.length
	});
	mocks.getDeviceGroup.mockImplementation(async (id: string) => ({
		group: protos.find((g) => (g.id?.value ?? '') === id),
		deviceIds: groups[id] ?? [],
		devices: []
	}));
}

const tiles = () => Array.from(document.querySelectorAll<HTMLElement>('[data-testid="fleet-tile"]'));

/** Click one bubble's tile by HOSTNAME — the grid is worst-first, so addressing
 *  by name keeps the test independent of that ordering. */
function clickTile(groupId: string, hostname: string) {
	const bubble = document.querySelector<HTMLElement>(`[data-group-id="${groupId}"]`)!;
	const tile = Array.from(
		bubble.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]')
	).find((t) => t.title.split(' · ')[0] === hostname);
	if (!tile) throw new Error(`no tile for ${hostname} in ${groupId}`);
	tile.click();
}

let consoleError: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
	document.body.innerHTML = '';
	mocks.url = new URL('http://localhost/devices');
	mocks.listDevices.mockReset();
	mocks.listDeviceGroups.mockReset();
	mocks.getDeviceGroup.mockReset();
	mocks.search.mockReset();
	mocks.rebootDevice.mockReset();
	mocks.setDeviceLabel.mockReset();
	mocks.goto.mockReset();
	mocks.pushState.mockReset();
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.rebootDevice.mockResolvedValue({});
	mocks.setDeviceLabel.mockResolvedValue({});
	toaster.success.mockReset();
	toaster.error.mockReset();
	consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
	resetShell();
	resetFleetSelection();
	setCarried(null);
});

afterEach(() => {
	consoleError.mockRestore();
});

describe('the resting pill stays quiet on the fleet', () => {
	// The caption is a CONTEXTUAL surface — draft eligibility, commit summaries,
	// conflict attribution — never an echo of stats the page already shows. The
	// fleet's summary strip counts these buckets on screen; captioning the pill
	// with the same numbers was duplication, not information.
	it('pushes no page-stat caption — the summary strip on the page is the one home for those numbers', async () => {
		respond(
			[
				device({ id: 'd1', hostname: 'api-01' }),
				device({ id: 'd2', hostname: 'api-02' }),
				device({ id: 'd3', hostname: 'api-03', compliance: ComplianceStatus.NON_COMPLIANT }),
				device({ id: 'd4', hostname: 'api-04', status: DeviceStatus.OFFLINE }),
				device({ id: 'd5', hostname: 'api-05', seen: null })
			],
			{ g1: ['d1', 'd2', 'd3', 'd4', 'd5'] }
		);

		const view = await render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(5));

		expect(pillMode()).toBe('nav');
		expect(pillSubtext(), 'no echo of the on-page stats').toBeNull();

		await view.unmount();
		expect(pillSubtext()).toBeNull();
	});

	it('a live selection still captions the pill, and clearing it leaves the caption empty', async () => {
		respond([device({ id: 'd1', hostname: 'api-01' }), device({ id: 'd2', hostname: 'api-02' })], {
			g1: ['d1', 'd2']
		});

		await render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(2));

		clickTile('g1', 'api-01');
		await vi.waitFor(() => expect(pillMode()).toBe('selection'));
		expect(pillSubtext()!.text).toBe('across 1 groups · 0 offline unavailable for live calls');

		clickTile('g1', 'api-01'); // toggle back off
		await vi.waitFor(() => expect(pillMode()).toBe('nav'));
		expect(pillSubtext()).toBeNull();
	});
});

describe('bulk reboot', () => {
	const fixture = () =>
		respond(
			[
				device({ id: 'd1', hostname: 'a1' }),
				device({ id: 'd2', hostname: 'a2', status: DeviceStatus.OFFLINE }),
				device({ id: 'd3', hostname: 'b1' })
			],
			{ alpha: ['d1', 'd2'], beta: ['d3'] }
		);

	async function selectAllThree() {
		await render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(3));
		clickTile('alpha', 'a1');
		clickTile('alpha', 'a2');
		clickTile('beta', 'b1');
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(3));
	}

	it('offers Assign, Reboot and Label in the concepts order', async () => {
		fixture();
		await selectAllThree();

		expect(shell.pill.selection!.actions.map((a) => a.id)).toEqual(['assign', 'reboot', 'label']);
		expect(shell.pill.selection!.actions.map((a) => a.primary === true)).toEqual([
			true,
			false,
			false
		]);
	});

	it('names the hosts and calls reboot once per id', async () => {
		fixture();
		await selectAllThree();

		runPillAction('reboot');

		const dialog = browser.getByRole('alertdialog');
		await expect.element(dialog).toBeVisible();
		const hosts = document.querySelector<HTMLElement>('[data-testid="bulk-reboot-hosts"]')!;
		// the unreachable member is named FIRST and is never dropped from the write
		expect(hosts.textContent!.replace(/\s+/g, ' ').trim()).toBe('a2 a1 b1');
		await dialog.getByTestId('bulk-reboot-confirm').click();

		await vi.waitFor(() => expect(mocks.rebootDevice).toHaveBeenCalledTimes(3));
		expect(mocks.rebootDevice.mock.calls.map((c) => c[0]).sort()).toEqual([
			'd1',
			'd2',
			'd3'
		]);
		await vi.waitFor(() =>
			expect(toaster.success).toHaveBeenCalledWith('Reboot requested on 3 devices')
		);
		expect(toaster.error).not.toHaveBeenCalled();
	});

	it('aggregates per-device failures into one toast and still writes every other device', async () => {
		fixture();
		mocks.rebootDevice.mockImplementation(async (id: string) => {
			if (id === 'd2') throw new Error('agent unreachable');
			return {};
		});
		await selectAllThree();

		runPillAction('reboot');
		await expect.element(browser.getByRole('alertdialog')).toBeVisible();
		await browser.getByTestId('bulk-reboot-confirm').click();

		// the failing device does NOT abort the queue behind it
		await vi.waitFor(() => expect(mocks.rebootDevice).toHaveBeenCalledTimes(3));
		await vi.waitFor(() =>
			expect(toaster.error).toHaveBeenCalledWith('2 requested, 1 failed')
		);
		expect(toaster.error).toHaveBeenCalledTimes(1); // one toast, not one per device
		expect(toaster.success).not.toHaveBeenCalled();
		expect(consoleError).toHaveBeenCalledWith(
			'fleet bulk: per-device failures',
			expect.arrayContaining([expect.objectContaining({ deviceId: 'd2', ok: false })])
		);
	});
});

describe('bulk label', () => {
	it('writes the entered key/value to every selected device', async () => {
		respond(
			[
				device({ id: 'd1', hostname: 'a1' }),
				device({ id: 'd2', hostname: 'a2', status: DeviceStatus.OFFLINE })
			],
			{ alpha: ['d1', 'd2'] }
		);

		await render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(2));
		clickTile('alpha', 'a1');
		clickTile('alpha', 'a2');
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(2));

		runPillAction('label');

		const dialog = browser.getByRole('dialog');
		await expect.element(dialog).toBeVisible();
		await dialog.getByLabelText('Key').fill('env');
		await dialog.getByLabelText('Value').fill('production');
		await dialog.getByTestId('add-label-confirm').click();

		await vi.waitFor(() => expect(mocks.setDeviceLabel).toHaveBeenCalledTimes(2));
		expect(mocks.setDeviceLabel.mock.calls.map((c) => [c[0], c[1], c[2]]).sort()).toEqual([
			['d1', 'env', 'production'],
			['d2', 'env', 'production']
		]);
		await vi.waitFor(() =>
			expect(toaster.success).toHaveBeenCalledWith('Label applied to 2 devices')
		);
	});
});
