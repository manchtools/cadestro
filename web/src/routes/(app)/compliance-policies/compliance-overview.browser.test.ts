// The compliance drill-down: the OVERVIEW is the landing level — the fleet as
// device tiles coloured by Device.complianceStatus (a ListDevices response
// field), with a policy chip row from ListCompliancePolicies. Status is never
// colour-alone (fleet legend grammar): violations carry the warning dot as a
// real child element, unknown renders hollow. A tile opens the device's
// existing compliance view; the policy list stays one zoom in.
//
// Per-policy tile FILTERING is deliberately absent: no list RPC carries
// per-device per-policy state (only per-device GetDeviceCompliancePolicyStatus
// does), and fanning that out across the fleet would fabricate a rollup.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ComplianceStatus } from '$sdk/powermanage/v1/common_pb';

const mocks = vi.hoisted(() => ({
	url: new URL('http://localhost/compliance-policies'),
	search: vi.fn(),
	listDevices: vi.fn(),
	listCompliancePolicies: vi.fn(),
	goto: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return mocks.url;
		},
		// The policy detail sheet reads its open-id from shallow-routing state.
		get state() {
			return {};
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
vi.mock('$lib/navigation', () => ({
	goto: (path: string) => mocks.goto(path),
	pushState: vi.fn(),
	replaceState: vi.fn()
}));
vi.mock('$lib/sdk', async () => {
	const common = await import('$sdk/powermanage/v1/common_pb');
	const control = await import('$sdk/powermanage/v1/control_pb');
	const actions = await import('$sdk/powermanage/v1/actions_pb');
	// The REAL pager: the sweep must actually walk the mocked list RPCs.
	const { fetchAllPages } = await import('$lib/sdk/paginate');
	return {
		...actions,
		...control,
		...common,
		apiClient: {
			search: mocks.search,
			listDevices: mocks.listDevices,
			listCompliancePolicies: mocks.listCompliancePolicies,
			deleteCompliancePolicy: vi.fn(),
			getCompliancePolicy: vi.fn(async () => ({ policy: null })),
			listActions: vi.fn(async () => ({ actions: [], nextPageToken: '' }))
		},
		formatTimestamp: () => '—',
		formatTimestampDateTime: () => '2026-01-01 00:00',
		fetchAllPages
	};
});

import CompliancePoliciesPage from './+page.svelte';

const DEVICES = [
	{ id: '01HZDEVCOMPLIANT000000000A', hostname: 'ok-01', complianceStatus: ComplianceStatus.COMPLIANT },
	{
		id: '01HZDEVVIOLATION000000000B',
		hostname: 'drift-01',
		complianceStatus: ComplianceStatus.NON_COMPLIANT
	},
	{ id: '01HZDEVUNKNOWN0000000000C', hostname: 'new-01', complianceStatus: ComplianceStatus.UNKNOWN }
];
const POLICY = {
	id: '01HZPOLICY000000000000000A',
	name: 'CIS baseline',
	description: '',
	rules: [],
	ruleCount: 2
};

const tiles = () =>
	Array.from(document.querySelectorAll<HTMLElement>('[data-testid="compliance-tile"]'));

beforeEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	mocks.url = new URL('http://localhost/compliance-policies');
	mocks.search.mockResolvedValue({ results: [], totalCount: 0 });
	mocks.listDevices.mockResolvedValue({ devices: DEVICES, nextPageToken: '', totalCount: 3 });
	mocks.listCompliancePolicies.mockResolvedValue({ policies: [POLICY], nextPageToken: '' });
});

describe('/compliance-policies — the overview is the landing level', () => {
	it('lands on the device-tile grid coloured by compliance state, never colour-alone', async () => {
		render(CompliancePoliciesPage);

		await vi.waitFor(() => expect(mocks.listDevices).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(() => expect(tiles()).toHaveLength(3), { timeout: 3000 });

		const byId = (id: string) => tiles().find((t) => t.dataset.deviceId === id)!;
		const compliant = byId(DEVICES[0].id);
		const violating = byId(DEVICES[1].id);
		const unknown = byId(DEVICES[2].id);

		expect(compliant.dataset.tone).toBe('ok');
		expect(violating.dataset.tone).toBe('warn');
		expect(unknown.dataset.tone).toBe('idle');
		// Never colour-alone: violations carry the dot as a REAL child element,
		// the others carry none; every tile names its status in words.
		expect(violating.querySelector('[data-marker="dot"]')).not.toBeNull();
		expect(compliant.querySelector('[data-marker="dot"]')).toBeNull();
		expect(compliant.getAttribute('aria-label')).toContain('Compliant');
		expect(violating.getAttribute('aria-label')).toContain('Non-Compliant');
		expect(unknown.getAttribute('aria-label')).toContain('Unknown');

		// The policy chip row comes from ListCompliancePolicies, counts included.
		const chip = document.querySelector<HTMLElement>('[data-testid="compliance-policy-chip"]');
		expect(chip).not.toBeNull();
		expect(chip!.textContent).toContain('CIS baseline');
		expect(chip!.textContent).toContain('2 rules');

		// The paused policy list never spends a Search RPC at the landing level.
		expect(mocks.search).not.toHaveBeenCalled();
	});

	it('a tile opens the device\'s existing compliance view', async () => {
		render(CompliancePoliciesPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(3), { timeout: 3000 });

		tiles()
			.find((t) => t.dataset.deviceId === DEVICES[1].id)!
			.click();

		expect(mocks.goto).toHaveBeenCalledWith(`/devices/${DEVICES[1].id}?tab=compliance`);
	});

	it('the level pill zooms to the existing policy list one level down', async () => {
		render(CompliancePoliciesPage);
		await vi.waitFor(() => expect(tiles()).toHaveLength(3), { timeout: 3000 });

		document.querySelector<HTMLButtonElement>('[data-testid="compliance-zoom-list"]')!.click();

		await vi.waitFor(() => expect(mocks.search).toHaveBeenCalled(), { timeout: 3000 });
		await vi.waitFor(
			() => expect(document.querySelector('[data-testid="row-list"]')).not.toBeNull(),
			{ timeout: 3000 }
		);
	});
});
