

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
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
	listAvailableActions: vi.fn(),
	goto: vi.fn(),
	pushState: vi.fn()
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
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

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
			deleteDevice: vi.fn(),
			assignDevice: vi.fn(),
			listAvailableActions: mocks.listAvailableActions,
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
import MyDevicesPage from '../my-devices/+page.svelte';
import { shell, resetShell, runPillAction, clearSelection } from '$lib/shell/shell.svelte';
import { getCarried, setCarried } from '$lib/shell/carried-selection.svelte';
import { resetFleetSelection } from './fleet-selection.svelte';

const MIN = 60;
const nowSec = () => Math.floor(Date.now() / 1000);

function device(o: {
	id: string;
	hostname: string;
	status?: DeviceStatus;
	compliance?: ComplianceStatus;

	seen?: number | null;
	syncIntervalMinutes?: number;
}) {
	return create(DeviceSchema, {
		id: { value: o.id },
		hostname: o.hostname,
		status: o.status ?? DeviceStatus.ONLINE,
		complianceStatus: o.compliance ?? ComplianceStatus.COMPLIANT,
		syncIntervalMinutes: o.syncIntervalMinutes ?? 0,
		lastSeenAt:
			o.seen === null
				? undefined
				: create(TimestampSchema, { seconds: BigInt(nowSec() - (o.seen ?? 0) * MIN) })
	});
}

function group(id: string, name: string, syncIntervalMinutes = 0) {
	return create(DeviceGroupSchema, { id: { value: id }, name, syncIntervalMinutes });
}

function respond(
	devices: ReturnType<typeof device>[],
	groups: Record<string, string[]> = {},
	groupProtos: ReturnType<typeof group>[] = []
) {
	mocks.listDevices.mockResolvedValue({
		devices,
		nextPageToken: '',
		totalCount: devices.length
	});
	const protos = groupProtos.length ? groupProtos : Object.keys(groups).map((id) => group(id, id));
	mocks.listDeviceGroups.mockResolvedValue({ groups: protos, nextPageToken: '', totalCount: protos.length });
	mocks.getDeviceGroup.mockImplementation(async (id: string) => ({
		group: protos.find((g) => (g.id?.value ?? '') === id),
		deviceIds: groups[id].map((value) => ({ value })),
		devices: []
	}));
}

const tiles = () => Array.from(document.querySelectorAll<HTMLElement>('[data-testid="fleet-tile"]'));
const rows = () => Array.from(document.querySelectorAll<HTMLElement>('[data-testid="fleet-row"]'));
const stats = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="fleet-stat"]')).map((s) => [
		s.dataset.tone,
		s.querySelector('b')!.textContent
	]);

function shiftClick(el: HTMLElement) {
	el.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, shiftKey: true }));
	el.click();
}

beforeEach(() => {
	document.body.innerHTML = '';
	mocks.url = new URL('http://localhost/devices');
	mocks.listDevices.mockReset();
	mocks.listDeviceGroups.mockReset();
	mocks.getDeviceGroup.mockReset();
	mocks.goto.mockReset();
	mocks.pushState.mockReset();
	mocks.search.mockReset();
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listAvailableActions.mockReset();
	mocks.listAvailableActions.mockResolvedValue([]);
	resetShell();
	resetFleetSelection();
	setCarried(null);
});

describe('fleet zoom — the tiles are the API answer', () => {
	it('encodes each status bucket as tone AND shape, and the summary strip agrees with them', async () => {
		respond(
			[
				device({ id: 'd-ok', hostname: 'api-01' }),
				device({ id: 'd-warn', hostname: 'api-02', compliance: ComplianceStatus.NON_COMPLIANT }),
				device({ id: 'd-crit', hostname: 'api-03', status: DeviceStatus.OFFLINE }),
				device({ id: 'd-idle', hostname: 'api-04', seen: null })
			],
			{ g1: ['d-ok', 'd-warn', 'd-crit', 'd-idle'] }
		);

		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(4));

		expect(tiles().map((t) => t.dataset.tone).sort()).toEqual(['crit', 'idle', 'ok', 'warn']);

		const byTone = Object.fromEntries(tiles().map((t) => [t.dataset.tone, t]));
		expect(byTone.ok.dataset.shape).toBe('none');
		expect(byTone.warn.querySelector('[data-marker="dot"]')).not.toBeNull();
		expect(byTone.crit.querySelector('[data-marker="notch"]')).not.toBeNull();
		expect(byTone.idle.dataset.shape).toBe('hollow');

		expect(document.querySelector('[data-marker="ring"]')).toBeNull();

		expect(stats()).toEqual([
			['ok', '1'],
			['warn', '1'],
			['crit', '1'],
			['idle', '1']
		]);
	});

	it('buckets decay against the group cadence, which outranks the device cadence', async () => {

		respond(
			[
				device({ id: 'd-slow', hostname: 'slow', seen: 45, syncIntervalMinutes: 60 }),
				device({ id: 'd-paced', hostname: 'paced', seen: 45, syncIntervalMinutes: 60 })
			],
			{ fast: ['d-paced'] },
			[group('fast', 'fast', 10)]
		);

		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(2));

		const byLabel = Object.fromEntries(tiles().map((t) => [t.title.split(' · ')[0], t]));
		expect(byLabel.slow.dataset.age).toBe('0');
		expect(byLabel.paced.dataset.age).toBe('2');
	});

	it('names hostname and status on every tile, so hover and assistive tech tell the same story', async () => {
		respond([device({ id: 'd-crit', hostname: 'web-prod-07', status: DeviceStatus.OFFLINE })], {
			g1: ['d-crit']
		});

		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(1));

		expect(tiles()[0].title).toBe('web-prod-07 · Critical');
	});

	it('shows the honest empty state, not an empty grid, when nothing is enrolled', async () => {
		respond([]);
		render(DevicesPage);

		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="fleet-empty"]')).not.toBeNull()
		);
		expect(document.body.textContent).toContain('No devices enrolled');
		expect(tiles().length).toBe(0);
	});

	it('carries the tour anchors the guided tour binds to', async () => {
		respond([device({ id: 'd1', hostname: 'a' })], { g1: ['d1'] });
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(1));

		for (const anchor of ['fleet-summary', 'fleet-grid', 'fleet-legend', 'fleet-zoom']) {
			expect(document.querySelector(`[data-tour="${anchor}"]`), anchor).not.toBeNull();
		}
	});
});

describe('group zoom — the near pane', () => {
	const fixture = () =>
		respond(
			[
				device({ id: 'd-ok', hostname: 'w-ok' }),
				device({ id: 'd-warn', hostname: 'w-warn', compliance: ComplianceStatus.NON_COMPLIANT }),
				device({ id: 'd-crit', hostname: 'w-crit', status: DeviceStatus.OFFLINE }),
				device({ id: 'd-idle', hostname: 'w-idle', seen: null })
			],
			{ 'web-prod': ['d-ok', 'd-warn', 'd-crit', 'd-idle'] }
		);

	it('lists worst-first and folds the healthy tail behind an expander', async () => {
		fixture();
		mocks.url = new URL('http://localhost/devices?zoom=group&group=web-prod');
		render(DevicesPage);

		await vi.waitFor(() => expect(rows().length).toBe(3));
		expect(rows().map((r) => r.dataset.deviceId)).toEqual(['d-crit', 'd-idle', 'd-warn']);
		expect(document.body.textContent).toContain('+ 1 healthy · collapsed');

		document.querySelector<HTMLButtonElement>('[data-testid="fleet-healthy-toggle"]')!.click();
		await vi.waitFor(() => expect(rows().length).toBe(4));
		expect(rows().at(-1)!.dataset.deviceId).toBe('d-ok');
	});

	it('keeps the far pane on screen as a pure summary beside the near pane', async () => {
		fixture();
		mocks.url = new URL('http://localhost/devices?zoom=group&group=web-prod');
		render(DevicesPage);

		await vi.waitFor(() => expect(rows().length).toBe(3));
		const bubble = document.querySelector<HTMLElement>('[data-testid="fleet-bubble"]');
		expect(bubble).not.toBeNull();
		expect(bubble!.dataset.focused).toBe('true');
	});

	it('opens a device window from the row quick action', async () => {
		fixture();
		mocks.url = new URL('http://localhost/devices?zoom=group&group=web-prod');
		render(DevicesPage);

		await vi.waitFor(() => expect(rows().length).toBe(3));
		rows()[0].querySelector<HTMLButtonElement>('[data-testid="fleet-row-open"]')!.click();

		await vi.waitFor(() => expect(shell.panels.length).toBe(1));
		expect(shell.panels[0]).toMatchObject({ kind: 'device', refId: 'd-crit', title: 'w-crit' });
	});
});

describe('zoom is URL state', () => {
	it('renders the level named by ?zoom and pushes the level the operator picks', async () => {
		respond([device({ id: 'd1', hostname: 'a' })], { g1: ['d1'] });
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(1));

		expect(document.querySelector('[data-testid="fleet-near-caption"]')).toBeNull();
		expect(
			document.querySelector<HTMLElement>('[data-testid="fleet-zoom-fleet"]')!.getAttribute('aria-pressed')
		).toBe('true');

		document.querySelector<HTMLButtonElement>('[data-testid="fleet-zoom-group"]')!.click();

		await vi.waitFor(() =>
			expect(document.querySelector('[data-testid="fleet-near-caption"]')).not.toBeNull()
		);

		expect(mocks.pushState).toHaveBeenCalledWith(
			expect.stringContaining('zoom=group&group=g1'),
			{}
		);
	});

	it('deep-links straight into the device level, which is the search-backed list', async () => {
		respond([device({ id: 'd1', hostname: 'a' })], { g1: ['d1'] });
		mocks.url = new URL('http://localhost/devices?zoom=device');
		render(DevicesPage);

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		expect(document.querySelector('[data-testid="fleet-bubble"]')).toBeNull();
	});

	it('never fires the search RPC at a level that does not render it', async () => {
		respond([device({ id: 'd1', hostname: 'a' })], { g1: ['d1'] });
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(1));

		await new Promise((r) => setTimeout(r, 500));
		expect(mocks.search).not.toHaveBeenCalled();
	});
});

describe('selection rides the pill into /assign', () => {
	const fixture = () =>
		respond(
			[
				device({ id: 'd1', hostname: 'a1' }),
				device({ id: 'd2', hostname: 'a2', status: DeviceStatus.OFFLINE }),
				device({ id: 'd3', hostname: 'b1' })
			],
			{ alpha: ['d1', 'd2'], beta: ['d3'] }
		);

	it('reports the real count, the groups it spans and unavailable members', async () => {
		fixture();
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		const alpha = document.querySelector<HTMLElement>('[data-group-id="alpha"]')!;
		alpha.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]')[0].click();

		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(1));
		expect(shell.pill.selection!.subtext).toBe('across 1 groups · 1 offline unavailable for live calls');
		expect(shell.pill.selection!.subtextTone).toBe('warn');

		document
			.querySelector<HTMLElement>('[data-group-id="beta"]')!
			.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]')[0]
			.click();

		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(2));
		expect(shell.pill.selection!.subtext).toBe('across 2 groups · 1 offline unavailable for live calls');
	});

	it('extends a range with shift-click, inside the clicked group only', async () => {
		fixture();
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		const alpha = document.querySelector<HTMLElement>('[data-group-id="alpha"]')!;
		const alphaTiles = alpha.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]');
		alphaTiles[0].click();
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(1));

		shiftClick(alphaTiles[1]);
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(2));

		shiftClick(
			document
				.querySelector<HTMLElement>('[data-group-id="beta"]')!
				.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]')[0]
		);
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(3));
	});

	it('hands the selected ids to the assign surface and navigates there', async () => {
		fixture();
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		document
			.querySelector<HTMLElement>('[data-group-id="beta"]')!
			.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]')[0]
			.click();
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(1));

		runPillAction('assign');

		expect(getCarried()).toEqual({ deviceIds: ['d3'], label: '1 devices' });
		expect(mocks.goto).toHaveBeenCalledWith('/assign');
	});

	it('keeps the selection alive for its own surface when another fleet mounts', async () => {
		fixture();
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(3));
		document
			.querySelector<HTMLElement>('[data-group-id="beta"]')!
			.querySelectorAll<HTMLButtonElement>('[data-testid="fleet-tile"]')[0]
			.click();
		await vi.waitFor(() => expect(shell.pill.selection?.count).toBe(1));

		mocks.url = new URL('http://localhost/my-devices');
		render(MyDevicesPage);
		await vi.waitFor(() => expect(mocks.listDevices).toHaveBeenCalledTimes(2));

		expect(shell.pill.selection?.count).toBe(1);
	});

	it('clearing from the pill drops the tile outlines too', async () => {
		fixture();
		render(DevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(3));

		const tile = document.querySelector<HTMLElement>('[data-group-id="beta"]')!
			.querySelector<HTMLButtonElement>('[data-testid="fleet-tile"]')!;
		tile.click();
		await vi.waitFor(() => expect(tile.dataset.selected).toBe('true'));

		clearSelection();

		await vi.waitFor(() => expect(shell.pill.selection).toBeNull());
		await vi.waitFor(() =>
			expect(
				document
					.querySelector<HTMLElement>('[data-group-id="beta"]')!
					.querySelector<HTMLElement>('[data-testid="fleet-tile"]')!.dataset.selected
			).toBeUndefined()
		);
	});
});

describe('my-devices — the same surface, constrained to the caller', () => {
	const fixture = () =>
		respond(
			[
				device({ id: 'mine-1', hostname: 'laptop-01' }),
				device({ id: 'mine-2', hostname: 'laptop-02', status: DeviceStatus.OFFLINE })
			],
			{ mine: ['mine-1', 'mine-2'] }
		);

	beforeEach(() => {
		mocks.url = new URL('http://localhost/my-devices');
	});

	it('reads ListDevices with my_devices_only and paints the same encoding', async () => {
		fixture();
		render(MyDevicesPage);
		await vi.waitFor(() => expect(tiles().length).toBe(2));

		expect(mocks.listDevices.mock.calls[0][4]).toBe(true);
		expect(tiles().map((t) => t.dataset.tone).sort()).toEqual(['crit', 'ok']);
		expect(stats()).toEqual([
			['ok', '1'],
			['warn', '0'],
			['crit', '1'],
			['idle', '0']
		]);
	});

	it('still drills into a device’s available actions from the row action', async () => {
		fixture();
		mocks.url = new URL('http://localhost/my-devices?zoom=group&group=mine');
		render(MyDevicesPage);
		await vi.waitFor(() => expect(rows().length).toBe(1));

		rows()[0].querySelector<HTMLButtonElement>('[data-testid="fleet-row-open"]')!.click();

		await vi.waitFor(() => expect(mocks.listAvailableActions).toHaveBeenCalledWith('mine-2'));
		expect(document.body.textContent).toContain('Available Actions');
	});
});
