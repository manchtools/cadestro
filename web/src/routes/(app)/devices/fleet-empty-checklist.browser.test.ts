// The getting-started checklist rides the EMPTY fleet — through the real seam.
//
// The card is not portalled, watched for or re-parented by anything: the devices
// route passes it to FleetSurface's `emptyExtra`, which is fleet-empty's own
// `extra` slot. So the contract these tests pin is structural — the checklist
// renders INSIDE `[data-testid="fleet-empty"]`, and a fleet that has devices
// never renders it at all.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { create } from '@bufbuild/protobuf';
import { DeviceSchema } from '$contract/cadestro/v1/control_pb';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/devices'),
	// fleet snapshot reads (./fleet-data) …
	listDevices: vi.fn(),
	listDeviceGroups: vi.fn(),
	getDeviceGroup: vi.fn(),
	search: vi.fn(),
	// … and the checklist's own probes ($lib/onboarding/checklist).
	listTokens: vi.fn(),
	listActions: vi.fn(),
	listAssignments: vi.fn(),
	listUsers: vi.fn(),
	listIdentityProviders: vi.fn()
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
	goto: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	afterNavigate: vi.fn(),
	beforeNavigate: vi.fn()
}));

// One faked client behind BOTH readers — the fleet sweep and the checklist
// probes are the same seam, so the page and the card agree on the same fleet.
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
			listTokens: mocks.listTokens,
			listActions: mocks.listActions,
			listAssignments: mocks.listAssignments,
			listUsers: mocks.listUsers,
			listIdentityProviders: mocks.listIdentityProviders,
			listAvailableActions: vi.fn().mockResolvedValue([])
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages: vi.fn().mockResolvedValue([])
	};
});

import DevicesPage from './+page.svelte';
import { resetOnboarding } from '$lib/onboarding';
import { resetShell } from '$lib/shell/shell.svelte';
import { resetFleetSelection } from './fleet-selection.svelte';

const empty = () => document.querySelector<HTMLElement>('[data-testid="fleet-empty"]');
const checklist = () => document.querySelector<HTMLElement>('[data-testid="onboarding-checklist"]');

// render() resolves to the result that carries unmount(); awaiting it keeps the
// page's own polling-free effects from outliving their test.
let mounted: { unmount: () => Promise<void> } | null = null;

beforeEach(() => {
	document.body.innerHTML = '';
	localStorage.clear();
	resetOnboarding();
	resetShell();
	resetFleetSelection();
	mocks.url = new URL('http://localhost/devices');
	for (const fn of Object.values(mocks)) if (typeof fn === 'function') fn.mockReset();

	mocks.listDevices.mockResolvedValue({ devices: [], nextPageToken: '', totalCount: 0 });
	mocks.listDeviceGroups.mockResolvedValue({ groups: [], nextPageToken: '', totalCount: 0 });
	mocks.getDeviceGroup.mockResolvedValue({ deviceIds: [], devices: [] });
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	// Every probe answers "nothing yet", so every row is a real, readable `todo`.
	mocks.listTokens.mockResolvedValue({ tokens: [] });
	mocks.listActions.mockResolvedValue({ actions: [] });
	mocks.listAssignments.mockResolvedValue({ assignments: [] });
	mocks.listUsers.mockResolvedValue({ users: [] });
	mocks.listIdentityProviders.mockResolvedValue({ providers: [] });
});

afterEach(async () => {
	await mounted?.unmount();
	mounted = null;
});

describe('the getting-started checklist rides the empty fleet', () => {
	it('renders inside the empty state, not merely somewhere on the page', async () => {
		mounted = await render(DevicesPage);

		await vi.waitFor(() => expect(empty()).not.toBeNull());
		await vi.waitFor(() => expect(checklist()).not.toBeNull());

		// The seam, asserted structurally: the card is a DESCENDANT of the empty
		// state — a portalled card sitting next to it would fail here.
		expect(empty()!.querySelector('[data-testid="onboarding-checklist"]')).not.toBeNull();
		expect(
			document.querySelectorAll('[data-testid="onboarding-checklist-row"]').length
		).toBe(5);
	});

	it('stays away from a fleet that has devices', async () => {
		mocks.listDevices.mockResolvedValue({
			devices: [create(DeviceSchema, { id: 'd1', hostname: 'api-01' })],
			nextPageToken: '',
			totalCount: 1
		});

		mounted = await render(DevicesPage);
		await vi.waitFor(() =>
			expect(document.querySelectorAll('[data-testid="fleet-tile"]').length).toBe(1)
		);

		// Long enough for the checklist's own onMount reads to have landed, had
		// anything mounted it.
		await new Promise((r) => setTimeout(r, 120));
		expect(empty()).toBeNull();
		expect(checklist()).toBeNull();
		expect(document.querySelector('[data-testid="onboarding-checklist-loading"]')).toBeNull();
		expect(mocks.listTokens).not.toHaveBeenCalled();
	});
});
